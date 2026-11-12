package rdp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tailscale.com/client/tailscale/apitype"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/client"
	pb "github.com/open-beagle/awecloud-signaling-desktop/pkg/proto"
)

// WhoIsClient 抽象 tsnet LocalClient.WhoIs 以便在单测中注入模拟节点身份
type WhoIsClient interface {
	WhoIs(ctx context.Context, remoteAddr string) (*apitype.WhoIsResponse, error)
}

type closeWriteConn interface {
	CloseWrite() error
}

type activeInboundSession struct {
	sessionUUID         string
	peerHeadscaleNodeID uint64
	sourceNodeID        uint64
	sourceUserName      string
	sourceDeviceName    string
	sourceIP            string
	grantID             uint64
	startedAt           time.Time
	remoteConn          net.Conn
	localConn           net.Conn
	forcedCloseReason   atomic.Value // string
	closeOnce           sync.Once
}

func (s *activeInboundSession) forceClose(reason string) {
	s.closeOnce.Do(func() {
		if reason != "" {
			s.forcedCloseReason.Store(reason)
		}
		if s.remoteConn != nil {
			_ = s.remoteConn.Close()
		}
		if s.localConn != nil {
			_ = s.localConn.Close()
		}
	})
}

func (s *activeInboundSession) reasonOrDefault(fallback string) string {
	if v := s.forcedCloseReason.Load(); v != nil {
		if str, ok := v.(string); ok && str != "" {
			return str
		}
	}
	return fallback
}

// InboundGate 负责被控端 tsnet:3389 按需入站拦截、节点级 WhoIs 门禁校验与活跃连接即时熔断
type InboundGate struct {
	mu               sync.RWMutex
	policy           *pb.DesktopInboundRDPPolicy
	localStatus      LocalRDPStatus
	tunnelConnected  bool
	lastExposedState bool

	whoIsProvider    func() (WhoIsClient, error)
	localDialer      func(ctx context.Context, network, addr string) (net.Conn, error)
	auditReporter    func(evt client.RDPSessionAuditEvent)
	onExposureChange func(localReady bool, localPort int, tunnelExposed bool)
	nowFunc          func() time.Time

	sessionsMu sync.Mutex
	sessions   map[string]*activeInboundSession

	rejectRateMu sync.Mutex
	lastRejectAt map[string]time.Time

	stopOnce sync.Once
	stopCh   chan struct{}
}

// InboundGateOptions 创建 InboundGate 的配置项
type InboundGateOptions struct {
	WhoIsProvider    func() (WhoIsClient, error)
	LocalDialer      func(ctx context.Context, network, addr string) (net.Conn, error)
	AuditReporter    func(evt client.RDPSessionAuditEvent)
	OnExposureChange func(localReady bool, localPort int, tunnelExposed bool)
	NowFunc          func() time.Time
}

// NewInboundGate 创建被控端 RDP 入站门禁
func NewInboundGate(opts InboundGateOptions) *InboundGate {
	dialer := opts.LocalDialer
	if dialer == nil {
		d := &net.Dialer{Timeout: 5 * time.Second}
		dialer = d.DialContext
	}
	nowFn := opts.NowFunc
	if nowFn == nil {
		nowFn = time.Now
	}

	g := &InboundGate{
		localStatus: LocalRDPStatus{
			LocalPort: DefaultRDPPort,
		},
		whoIsProvider:    opts.WhoIsProvider,
		localDialer:      dialer,
		auditReporter:    opts.AuditReporter,
		onExposureChange: opts.OnExposureChange,
		nowFunc:          nowFn,
		sessions:         make(map[string]*activeInboundSession),
		lastRejectAt:     make(map[string]time.Time),
		stopCh:           make(chan struct{}),
	}
	go g.leaseWatchLoop()
	return g
}

// Stop 停止门禁后台协程并强制断开全部存量入站连接
func (g *InboundGate) Stop() {
	g.stopOnce.Do(func() {
		close(g.stopCh)
		g.mu.Lock()
		g.tunnelConnected = false
		g.policy = nil
		g.mu.Unlock()
		g.closeAllSessions("tunnel_stopped")
		g.notifyExposureIfChanged()
	})
}

func (g *InboundGate) leaseWatchLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-g.stopCh:
			return
		case <-ticker.C:
			g.reconcileActiveSessions()
			g.notifyExposureIfChanged()
		}
	}
}

// SetTunnelConnected 更新隧道在线状态
func (g *InboundGate) SetTunnelConnected(connected bool) {
	g.mu.Lock()
	g.tunnelConnected = connected
	g.mu.Unlock()

	if !connected {
		g.closeAllSessions("tunnel_stopped")
	}
	g.notifyExposureIfChanged()
}

// UpdateLocalStatus 更新本机零握手 RDP 探测状态
func (g *InboundGate) UpdateLocalStatus(status LocalRDPStatus) {
	if status.LocalPort <= 0 {
		status.LocalPort = DefaultRDPPort
	}
	g.mu.Lock()
	g.localStatus = status
	g.mu.Unlock()

	g.reconcileActiveSessions()
	g.notifyExposureIfChanged()
}

// ApplyPolicy 应用 Server 下发的最新入站门禁白名单快照，并对已失效会话立即执行强制断流
func (g *InboundGate) ApplyPolicy(policy *pb.DesktopInboundRDPPolicy) {
	g.mu.Lock()
	g.policy = policy
	g.mu.Unlock()

	g.reconcileActiveSessions()
	g.notifyExposureIfChanged()
}

// IsCurrentlyExposed 判断当前本机是否正处于允许入站隧道 3389 转发的有效状态
func (g *InboundGate) IsCurrentlyExposed() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.computeExposedLocked(g.nowFunc())
}

func (g *InboundGate) computeExposedLocked(now time.Time) bool {
	if !g.tunnelConnected || g.policy == nil {
		return false
	}
	if !g.policy.EffectiveEnabled || g.policy.AdminDisabled {
		return false
	}
	if g.policy.ValidUntilUnix > 0 && now.Unix() > g.policy.ValidUntilUnix {
		return false
	}
	mode := strings.ToLower(strings.TrimSpace(g.policy.RdpMode))
	if mode == "disabled" {
		return false
	}
	if mode == "enabled" {
		return g.localStatus.PortListening || g.localStatus.Ready
	}
	return g.localStatus.Ready
}

func (g *InboundGate) notifyExposureIfChanged() {
	g.mu.Lock()
	now := g.nowFunc()
	exposed := g.computeExposedLocked(now)
	localReady := g.localStatus.Ready
	localPort := g.localStatus.LocalPort
	if localPort <= 0 {
		localPort = DefaultRDPPort
	}
	changed := g.lastExposedState != exposed
	g.lastExposedState = exposed
	cb := g.onExposureChange
	g.mu.Unlock()

	if changed && cb != nil {
		cb(localReady, localPort, exposed)
	}
}

// ActiveSessionCount 返回当前正在转发中的入站 RDP 会话数
func (g *InboundGate) ActiveSessionCount() int {
	g.sessionsMu.Lock()
	defer g.sessionsMu.Unlock()
	return len(g.sessions)
}

// FallbackTCPHandler 供 tsnet.Server.RegisterFallbackTCPHandler 注册的按需入站回调
func (g *InboundGate) FallbackTCPHandler(src, dst netip.AddrPort) (func(net.Conn), bool) {
	if dst.Port() != DefaultRDPPort {
		return nil, false
	}
	if !g.IsCurrentlyExposed() {
		return nil, false
	}
	return func(conn net.Conn) {
		g.handleInboundConn(src, conn)
	}, true
}

func (g *InboundGate) handleInboundConn(src netip.AddrPort, remoteConn net.Conn) {
	now := g.nowFunc()

	g.mu.RLock()
	exposed := g.computeExposedLocked(now)
	policy := g.policy
	localPort := g.localStatus.LocalPort
	localPortListening := g.localStatus.PortListening
	g.mu.RUnlock()

	if !exposed || policy == nil {
		_ = remoteConn.Close()
		return
	}

	targetPort := int(policy.TargetPort)
	if localPort > 0 {
		targetPort = localPort
	}
	if targetPort <= 0 || targetPort > 65535 {
		targetPort = DefaultRDPPort
	}

	peer, rejectReason := g.authorizePeer(src, policy, now)
	if peer == nil {
		log.Printf("[RDP Inbound] 拒绝未授权入站连接 src=%s: %s", src.String(), rejectReason)
		_ = remoteConn.Close()
		g.emitRejectedAuditRateLimited(nil, src.Addr().String(), rejectReason, now)
		return
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	localAddr := fmt.Sprintf("127.0.0.1:%d", targetPort)
	localConn, err := g.localDialer(dialCtx, "tcp", localAddr)
	cancel()
	if err != nil {
		if localPortListening && isDialTimeoutError(err) {
			log.Printf("[RDP Inbound] 连接本机 RDP 回环地址超时 (%s, PortListening=true): %v；请检查 Windows Defender 防火墙规则 RemoteDesktop-UserMode-In-TCP 的 RemoteAddress 是否包含 127.0.0.1（或 Any）", localAddr, err)
			g.emitRejectedAuditRateLimited(peer, src.Addr().String(), "local_loopback_timeout", now)
		} else {
			log.Printf("[RDP Inbound] 连接本机 RDP 服务失败 (%s): %v", localAddr, err)
			g.emitRejectedAuditRateLimited(peer, src.Addr().String(), "local_dial_failed", now)
		}
		_ = remoteConn.Close()
		return
	}

	sessionID := newRDPSessionUUID()
	sess := &activeInboundSession{
		sessionUUID:         sessionID,
		peerHeadscaleNodeID: peer.HeadscaleNodeId,
		sourceNodeID:        peer.SourceNodeId,
		sourceUserName:      peer.SourceUserName,
		sourceDeviceName:    peer.SourceDeviceName,
		sourceIP:            src.Addr().String(),
		grantID:             peer.GrantId,
		startedAt:           now,
		remoteConn:          remoteConn,
		localConn:           localConn,
	}

	g.sessionsMu.Lock()
	g.sessions[sessionID] = sess
	g.sessionsMu.Unlock()

	// 入库后再校验一次策略，防止建连窗口期刚发生撤销漏网
	if ok, reason := g.isSessionStillAllowed(sess, g.nowFunc()); !ok {
		sess.forceClose(reason)
		g.sessionsMu.Lock()
		delete(g.sessions, sessionID)
		g.sessionsMu.Unlock()
		return
	}

	log.Printf("[RDP Inbound] 放行入站远程桌面会话 session=%s peerUser=%s peerDevice=%s hsNode=%d -> %s",
		sessionID, sess.sourceUserName, sess.sourceDeviceName, sess.peerHeadscaleNodeID, localAddr)

	g.emitAuditAsync(client.RDPSessionAuditEvent{
		SessionUUID:           sessionID,
		EventType:             "started",
		SourceHeadscaleNodeID: sess.peerHeadscaleNodeID,
		SourceNodeID:          sess.sourceNodeID,
		SourceUserName:        sess.sourceUserName,
		SourceDeviceName:      sess.sourceDeviceName,
		SourceIP:              sess.sourceIP,
		GrantID:               sess.grantID,
		TimestampUnix:         sess.startedAt.Unix(),
	})

	var copyWG sync.WaitGroup
	copyWG.Add(2)

	go func() {
		defer copyWG.Done()
		_, _ = io.Copy(localConn, remoteConn)
		if cw, ok := localConn.(closeWriteConn); ok {
			_ = cw.CloseWrite()
		} else {
			_ = localConn.Close()
		}
	}()

	go func() {
		defer copyWG.Done()
		_, _ = io.Copy(remoteConn, localConn)
		if cw, ok := remoteConn.(closeWriteConn); ok {
			_ = cw.CloseWrite()
		} else {
			_ = remoteConn.Close()
		}
	}()

	copyWG.Wait()
	sess.forceClose("normal_eof")

	g.sessionsMu.Lock()
	delete(g.sessions, sessionID)
	g.sessionsMu.Unlock()

	endedAt := g.nowFunc()
	durationMs := endedAt.Sub(sess.startedAt).Milliseconds()
	if durationMs < 0 {
		durationMs = 0
	}
	closeReason := sess.reasonOrDefault("normal_eof")
	log.Printf("[RDP Inbound] 远程桌面会话结束 session=%s reason=%s duration=%dms", sessionID, closeReason, durationMs)

	g.emitAuditAsync(client.RDPSessionAuditEvent{
		SessionUUID:           sessionID,
		EventType:             "ended",
		SourceHeadscaleNodeID: sess.peerHeadscaleNodeID,
		SourceNodeID:          sess.sourceNodeID,
		SourceUserName:        sess.sourceUserName,
		SourceDeviceName:      sess.sourceDeviceName,
		SourceIP:              sess.sourceIP,
		GrantID:               sess.grantID,
		CloseReason:           closeReason,
		DurationMs:            durationMs,
		TimestampUnix:         endedAt.Unix(),
	})
}

func (g *InboundGate) authorizePeer(src netip.AddrPort, policy *pb.DesktopInboundRDPPolicy, now time.Time) (*pb.AllowedRDPPeer, string) {
	if g.whoIsProvider == nil {
		return nil, "whois_unavailable"
	}
	whoIsClient, err := g.whoIsProvider()
	if err != nil || whoIsClient == nil {
		return nil, "whois_client_error"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	whois, err := whoIsClient.WhoIs(ctx, src.String())
	if err != nil || whois == nil || whois.Node == nil {
		return nil, "whois_lookup_failed"
	}

	peerHSNodeID := uint64(whois.Node.ID)
	if peerHSNodeID == 0 {
		return nil, "invalid_peer_node_id"
	}

	var matched *pb.AllowedRDPPeer
	for _, candidate := range policy.AllowedPeers {
		if candidate != nil && candidate.HeadscaleNodeId == peerHSNodeID {
			matched = candidate
			break
		}
	}
	if matched == nil {
		return nil, fmt.Sprintf("peer_node_%d_not_in_allowlist", peerHSNodeID)
	}

	if matched.ExpiresAtUnix > 0 && now.Unix() > matched.ExpiresAtUnix {
		return nil, "grant_expired"
	}

	if expectedHSUser := strings.TrimSpace(matched.HeadscaleUserName); expectedHSUser != "" {
		if whois.UserProfile == nil || !matchesHeadscaleLoginName(whois.UserProfile.LoginName, expectedHSUser) {
			return nil, "headscale_user_mismatch"
		}
	}

	return matched, ""
}

func matchesHeadscaleLoginName(actualLogin, expectedUser string) bool {
	actual := strings.ToLower(strings.TrimSpace(actualLogin))
	expected := strings.ToLower(strings.TrimSpace(expectedUser))
	if actual == "" || expected == "" {
		return false
	}
	if actual == expected {
		return true
	}
	return strings.HasPrefix(actual, expected+"@")
}

func (g *InboundGate) isSessionStillAllowed(sess *activeInboundSession, now time.Time) (bool, string) {
	g.mu.RLock()
	exposed := g.computeExposedLocked(now)
	policy := g.policy
	g.mu.RUnlock()

	if !exposed || policy == nil {
		if policy != nil && policy.ValidUntilUnix > 0 && now.Unix() > policy.ValidUntilUnix {
			return false, "policy_lease_expired"
		}
		if policy != nil && policy.AdminDisabled {
			return false, "admin_disabled"
		}
		return false, "policy_disabled"
	}

	for _, peer := range policy.AllowedPeers {
		if peer == nil || peer.HeadscaleNodeId != sess.peerHeadscaleNodeID {
			continue
		}
		if peer.GrantId != sess.grantID {
			continue
		}
		if peer.ExpiresAtUnix > 0 && now.Unix() > peer.ExpiresAtUnix {
			return false, "grant_expired"
		}
		return true, ""
	}
	return false, "policy_revoked"
}

func (g *InboundGate) reconcileActiveSessions() {
	now := g.nowFunc()

	g.sessionsMu.Lock()
	snapshot := make([]*activeInboundSession, 0, len(g.sessions))
	for _, sess := range g.sessions {
		snapshot = append(snapshot, sess)
	}
	g.sessionsMu.Unlock()

	for _, sess := range snapshot {
		if ok, reason := g.isSessionStillAllowed(sess, now); !ok {
			log.Printf("[RDP Inbound] 强制断开已失效远程桌面会话 session=%s reason=%s", sess.sessionUUID, reason)
			sess.forceClose(reason)
		}
	}
}

func (g *InboundGate) closeAllSessions(reason string) {
	g.sessionsMu.Lock()
	snapshot := make([]*activeInboundSession, 0, len(g.sessions))
	for _, sess := range g.sessions {
		snapshot = append(snapshot, sess)
	}
	g.sessionsMu.Unlock()

	for _, sess := range snapshot {
		sess.forceClose(reason)
	}
}

func isDialTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func (g *InboundGate) emitRejectedAuditRateLimited(peer *pb.AllowedRDPPeer, sourceIP, reason string, now time.Time) {
	rateKey := sourceIP + "|" + reason
	g.rejectRateMu.Lock()
	last, exists := g.lastRejectAt[rateKey]
	if exists && now.Sub(last) < 15*time.Second {
		g.rejectRateMu.Unlock()
		return
	}
	g.lastRejectAt[rateKey] = now
	g.rejectRateMu.Unlock()

	evt := client.RDPSessionAuditEvent{
		SessionUUID:   newRDPSessionUUID(),
		EventType:     "rejected",
		SourceIP:      sourceIP,
		CloseReason:   reason,
		TimestampUnix: now.Unix(),
	}
	if peer != nil {
		evt.SourceHeadscaleNodeID = peer.HeadscaleNodeId
		evt.SourceNodeID = peer.SourceNodeId
		evt.SourceUserName = peer.SourceUserName
		evt.SourceDeviceName = peer.SourceDeviceName
		evt.GrantID = peer.GrantId
	}
	g.emitAuditAsync(evt)
}

func (g *InboundGate) emitAuditAsync(evt client.RDPSessionAuditEvent) {
	if g.auditReporter == nil {
		return
	}
	go g.auditReporter(evt)
}

func newRDPSessionUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("rdp-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
