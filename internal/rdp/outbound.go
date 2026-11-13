package rdp

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/netutil"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/vip"
)

const (
	DefaultOutboundPortBase = 13389
	DefaultOutboundPortMax  = 13999
	defaultIdleRouteTTL     = 30 * time.Minute
)

// DialFunc 通过 tsnet 隧道拨号远端被控节点 3389 的函数签名
type DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// RDPEndpointInfo 主控端本地按需代理分配结果
type RDPEndpointInfo struct {
	NodeID            uint64 `json:"node_id"`
	DeviceName        string `json:"device_name"`
	RemoteIP          string `json:"remote_ip"`
	Domain            string `json:"domain"`
	VIP               string `json:"vip"`
	VIPPort           int    `json:"vip_port"`
	LocalAddr         string `json:"local_addr"`
	LocalPort         int    `json:"local_port"`
	RecommendedTarget string `json:"recommended_target"`
}

// RDPConnectResult 一键发起远程桌面连接的返回结果
type RDPConnectResult struct {
	Endpoint     RDPEndpointInfo `json:"endpoint"`
	Launched     bool            `json:"launched"`
	LaunchMethod string          `json:"launch_method"`
	RDPFilePath  string          `json:"rdp_file_path,omitempty"`
	Message      string          `json:"message"`
}

type outboundRoute struct {
	nodeID       uint64
	deviceName   string
	remoteIP     string
	domain       string
	vipAddr      string
	vipPort      int
	localPort    int
	localLn      net.Listener
	vipLn        net.Listener
	activeConns  atomic.Int64
	lastActiveNs atomic.Int64
	stopOnce     sync.Once
	stopCh       chan struct{}
}

func (r *outboundRoute) touch(now time.Time) {
	r.lastActiveNs.Store(now.UnixNano())
}

func (r *outboundRoute) lastActive() time.Time {
	ns := r.lastActiveNs.Load()
	if ns <= 0 {
		return time.Time{}
	}
	return time.Unix(0, ns)
}

func (r *outboundRoute) close() {
	r.stopOnce.Do(func() {
		close(r.stopCh)
		if r.localLn != nil {
			_ = r.localLn.Close()
		}
		if r.vipLn != nil {
			_ = r.vipLn.Close()
		}
	})
}

// OutboundManager 管理主控端本地按需 127.0.0.1:<13389+> 与个人主机域名 VIP 代理生命周期
type OutboundManager struct {
	dial         DialFunc
	vipAllocator *vip.Allocator
	getSavedPort func(nodeID uint64) int
	savePort     func(nodeID uint64, port int)
	verifyVIPFn  func(domain string, expectedVIP string) bool
	idleTTL      time.Duration
	nowFunc      func() time.Time

	mu           sync.RWMutex
	routesByNode map[uint64]*outboundRoute
	domainToVIP  map[string]string
	stopOnce     sync.Once
	stopCh       chan struct{}
}

// OutboundManagerOptions 主控端代理管理器配置项
type OutboundManagerOptions struct {
	Dial         DialFunc
	VIPAllocator *vip.Allocator
	GetSavedPort func(nodeID uint64) int
	SavePort     func(nodeID uint64, port int)
	VerifyVIP    func(domain string, expectedVIP string) bool
	IdleTTL      time.Duration
	NowFunc      func() time.Time
}

// NewOutboundManager 创建主控端按需本地 RDP 代理管理器
func NewOutboundManager(opts OutboundManagerOptions) *OutboundManager {
	idleTTL := opts.IdleTTL
	if idleTTL <= 0 {
		idleTTL = defaultIdleRouteTTL
	}
	nowFn := opts.NowFunc
	if nowFn == nil {
		nowFn = time.Now
	}
	verifyFn := opts.VerifyVIP
	if verifyFn == nil {
		verifyFn = defaultOSVerifyDomainVIP
	}
	m := &OutboundManager{
		dial:         opts.Dial,
		vipAllocator: opts.VIPAllocator,
		getSavedPort: opts.GetSavedPort,
		savePort:     opts.SavePort,
		verifyVIPFn:  verifyFn,
		idleTTL:      idleTTL,
		nowFunc:      nowFn,
		routesByNode: make(map[uint64]*outboundRoute),
		domainToVIP:  make(map[string]string),
		stopCh:       make(chan struct{}),
	}
	go m.reapIdleLoop()
	return m
}

// StopAll 关闭全部主控端本地代理监听并释放 VIP 映射
func (m *OutboundManager) StopAll() {
	m.stopOnce.Do(func() {
		close(m.stopCh)
	})

	m.mu.Lock()
	routes := make([]*outboundRoute, 0, len(m.routesByNode))
	for id, r := range m.routesByNode {
		routes = append(routes, r)
		delete(m.routesByNode, id)
	}
	m.domainToVIP = make(map[string]string)
	m.mu.Unlock()

	for _, r := range routes {
		r.close()
		if m.vipAllocator != nil && r.domain != "" {
			m.vipAllocator.Release(r.domain)
		}
	}
}

// ResolveVIP 供本地 DNS 服务器直接解析已注册的 <device>.rdp.beagle 域名
func (m *OutboundManager) ResolveVIP(domain string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	if normalized == "" {
		return "", false
	}
	m.mu.RLock()
	vipAddr, ok := m.domainToVIP[normalized]
	m.mu.RUnlock()
	return vipAddr, ok && vipAddr != ""
}

// EnsureEndpoint 按需为指定远端设备创建或复用无冲突本地代理入口（兼容旧签名）
func (m *OutboundManager) EnsureEndpoint(nodeID uint64, deviceName string, remoteIP string) (RDPEndpointInfo, error) {
	return m.EnsureEndpointWithDomain(nodeID, deviceName, remoteIP, "")
}

// EnsureEndpointWithDomain 按需为指定远端设备创建或复用无冲突本地代理入口（优先采用三段式个人主机域名）
func (m *OutboundManager) EnsureEndpointWithDomain(nodeID uint64, deviceName string, remoteIP string, preferredDomain string) (RDPEndpointInfo, error) {
	remoteIP = strings.TrimSpace(remoteIP)
	if nodeID == 0 {
		return RDPEndpointInfo{}, fmt.Errorf("缺少目标设备标识")
	}
	if remoteIP == "" {
		return RDPEndpointInfo{}, fmt.Errorf("目标设备隧道 IP 未就绪")
	}
	if m.dial == nil {
		return RDPEndpointInfo{}, fmt.Errorf("隧道拨号器未初始化")
	}

	domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(preferredDomain), "."))
	if domain == "" {
		domain = BuildRDPDomain(deviceName, nodeID)
	}
	now := m.nowFunc()

	m.mu.Lock()
	if existing, ok := m.routesByNode[nodeID]; ok {
		if existing.remoteIP == remoteIP && existing.domain == domain {
			existing.touch(now)
			m.mu.Unlock()
			return m.routeToEndpointInfo(existing), nil
		}
		delete(m.routesByNode, nodeID)
		delete(m.domainToVIP, existing.domain)
		existing.close()
		if m.vipAllocator != nil && existing.domain != "" {
			m.vipAllocator.Release(existing.domain)
		}
	}
	m.mu.Unlock()

	preferredPort := 0
	if m.getSavedPort != nil {
		preferredPort = m.getSavedPort(nodeID)
	}

	localLn, boundPort, err := bindConflictFreeLoopbackPort(preferredPort)
	if err != nil {
		return RDPEndpointInfo{}, err
	}
	if m.savePort != nil && boundPort != preferredPort {
		m.savePort(nodeID, boundPort)
	}

	var vipAddr string
	var vipPort int
	var vipLn net.Listener
	if m.vipAllocator != nil {
		if allocated, allocErr := m.vipAllocator.Allocate(domain); allocErr == nil && allocated != "" {
			if ln, lnErr := net.Listen("tcp", fmt.Sprintf("%s:%d", allocated, DefaultRDPPort)); lnErr == nil {
				vipAddr = allocated
				vipPort = DefaultRDPPort
				vipLn = ln
			} else if lnFallback, fbErr := net.Listen("tcp", fmt.Sprintf("%s:%d", allocated, boundPort)); fbErr == nil {
				vipAddr = allocated
				vipPort = boundPort
				vipLn = lnFallback
				log.Printf("[RDP Outbound] 本机 3389 已被系统服务占用，VIP 绑定至 %s:%d (domain=%s)", allocated, boundPort, domain)
			} else {
				log.Printf("[RDP Outbound] VIP %s 监听失败，回退本地 127.0.0.1:%d: %v", allocated, boundPort, lnErr)
				m.vipAllocator.Release(domain)
			}
		}
	}

	route := &outboundRoute{
		nodeID:     nodeID,
		deviceName: deviceName,
		remoteIP:   remoteIP,
		domain:     domain,
		vipAddr:    vipAddr,
		vipPort:    vipPort,
		localPort:  boundPort,
		localLn:    localLn,
		vipLn:      vipLn,
		stopCh:     make(chan struct{}),
	}
	route.touch(now)

	m.mu.Lock()
	if raced, exists := m.routesByNode[nodeID]; exists {
		m.mu.Unlock()
		route.close()
		if m.vipAllocator != nil && vipAddr != "" {
			m.vipAllocator.Release(domain)
		}
		raced.touch(now)
		return m.routeToEndpointInfo(raced), nil
	}
	m.routesByNode[nodeID] = route
	if vipAddr != "" {
		m.domainToVIP[domain] = vipAddr
	}
	m.mu.Unlock()

	go m.acceptLoop(route, localLn)
	if vipLn != nil {
		go m.acceptLoop(route, vipLn)
	}

	info := m.routeToEndpointInfo(route)
	log.Printf("[RDP Outbound] 已建立本地远程桌面入口: target=%s (local=%s, domain=%s, vip=%s:%d) -> %s:%d",
		info.RecommendedTarget, info.LocalAddr, info.Domain, info.VIP, info.VIPPort, remoteIP, DefaultRDPPort)
	return info, nil
}

// RefreshEndpoint 在系统 DNS / hosts 完成自愈同步后重新核验并返回指定节点的最新推荐连接地址
func (m *OutboundManager) RefreshEndpoint(nodeID uint64) (RDPEndpointInfo, bool) {
	m.mu.RLock()
	route, ok := m.routesByNode[nodeID]
	m.mu.RUnlock()
	if !ok || route == nil {
		return RDPEndpointInfo{}, false
	}
	route.touch(m.nowFunc())
	info := m.routeToEndpointInfo(route)
	log.Printf("[RDP Outbound] 已刷新本地远程桌面入口: target=%s (local=%s, domain=%s, vip=%s:%d)",
		info.RecommendedTarget, info.LocalAddr, info.Domain, info.VIP, info.VIPPort)
	return info, true
}

func (m *OutboundManager) routeToEndpointInfo(r *outboundRoute) RDPEndpointInfo {
	localAddr := fmt.Sprintf("127.0.0.1:%d", r.localPort)
	recommended := localAddr
	if r.domain != "" && r.vipAddr != "" && r.vipPort > 0 && m.verifyVIPFn != nil && m.verifyVIPFn(r.domain, r.vipAddr) {
		if r.vipPort == DefaultRDPPort {
			recommended = r.domain
		} else {
			recommended = fmt.Sprintf("%s:%d", r.domain, r.vipPort)
		}
	}
	return RDPEndpointInfo{
		NodeID:            r.nodeID,
		DeviceName:        r.deviceName,
		RemoteIP:          r.remoteIP,
		Domain:            r.domain,
		VIP:               r.vipAddr,
		VIPPort:           r.vipPort,
		LocalAddr:         localAddr,
		LocalPort:         r.localPort,
		RecommendedTarget: recommended,
	}
}

func defaultOSVerifyDomainVIP(domain string, expectedVIP string) bool {
	if domain == "" || expectedVIP == "" {
		return false
	}
	resolver := &net.Resolver{PreferGo: false}
	for attempt := 0; attempt < 2; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
		ips, err := resolver.LookupIPAddr(ctx, domain)
		cancel()
		if err == nil {
			for _, item := range ips {
				if item.IP != nil && item.IP.String() == expectedVIP {
					return true
				}
			}
		}
		if attempt == 0 {
			time.Sleep(40 * time.Millisecond)
		}
	}
	return false
}

func bindConflictFreeLoopbackPort(preferred int) (net.Listener, int, error) {
	if preferred >= 1024 && preferred <= 65535 && preferred != DefaultRDPPort {
		if ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", preferred)); err == nil {
			return ln, preferred, nil
		}
	}

	for port := DefaultOutboundPortBase; port <= DefaultOutboundPortMax; port++ {
		if port == preferred {
			continue
		}
		if ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port)); err == nil {
			return ln, port, nil
		}
	}

	// 若预设池全部被系统排除端口占用，退回内核动态分配空闲端口
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, 0, fmt.Errorf("无法分配本地空闲代理端口: %w", err)
	}
	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		_ = ln.Close()
		return nil, 0, fmt.Errorf("解析本地代理监听地址失败")
	}
	return ln, tcpAddr.Port, nil
}

func (m *OutboundManager) acceptLoop(route *outboundRoute, ln net.Listener) {
	for {
		clientConn, err := ln.Accept()
		if err != nil {
			select {
			case <-route.stopCh:
				return
			case <-m.stopCh:
				return
			default:
				return
			}
		}
		go m.forwardOutboundConn(route, clientConn)
	}
}

func (m *OutboundManager) forwardOutboundConn(route *outboundRoute, clientConn net.Conn) {
	route.activeConns.Add(1)
	route.touch(m.nowFunc())
	defer func() {
		route.activeConns.Add(-1)
		route.touch(m.nowFunc())
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	targetAddr := net.JoinHostPort(route.remoteIP, fmt.Sprintf("%d", DefaultRDPPort))
	remoteConn, err := m.dial(ctx, "tcp", targetAddr)
	cancel()
	if err != nil {
		log.Printf("[RDP Outbound] 隧道连接远端 %s 失败: %v", targetAddr, err)
		_ = clientConn.Close()
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = io.Copy(remoteConn, clientConn)
		if cw, ok := remoteConn.(closeWriteConn); ok {
			_ = cw.CloseWrite()
		} else {
			_ = remoteConn.Close()
		}
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(clientConn, remoteConn)
		if cw, ok := clientConn.(closeWriteConn); ok {
			_ = cw.CloseWrite()
		} else {
			_ = clientConn.Close()
		}
	}()

	wg.Wait()
	_ = clientConn.Close()
	_ = remoteConn.Close()
}

func (m *OutboundManager) reapIdleLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.ReapIdleRoutes()
		}
	}
}

// ReapIdleRoutes 回收超过空闲阈值且无活跃连接的本地代理监听
func (m *OutboundManager) ReapIdleRoutes() int {
	now := m.nowFunc()
	var expired []*outboundRoute

	m.mu.Lock()
	for id, r := range m.routesByNode {
		if r.activeConns.Load() == 0 && !r.lastActive().IsZero() && now.Sub(r.lastActive()) > m.idleTTL {
			expired = append(expired, r)
			delete(m.routesByNode, id)
			delete(m.domainToVIP, r.domain)
		}
	}
	m.mu.Unlock()

	for _, r := range expired {
		r.close()
		if m.vipAllocator != nil && r.domain != "" {
			m.vipAllocator.Release(r.domain)
		}
		log.Printf("[RDP Outbound] 已回收空闲本地代理入口: %s (node=%d)", r.domain, r.nodeID)
	}
	return len(expired)
}

// BuildRDPDomain 生成合法且唯一定位的设备远程桌面域名 <sanitized-name>-<node-id>.rdp.beagle
func BuildRDPDomain(deviceName string, nodeID uint64) string {
	var b strings.Builder
	lastHyphen := false
	for _, r := range strings.ToLower(strings.TrimSpace(deviceName)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastHyphen = false
		case unicode.IsSpace(r) || r == '-' || r == '_' || r == '.':
			if b.Len() > 0 && !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
		if b.Len() >= 36 {
			break
		}
	}
	prefix := strings.Trim(b.String(), "-")
	if prefix == "" {
		prefix = "desktop"
	}
	suffix := netutil.ActiveDomainSuffix()
	if suffix == "" {
		suffix = ".beagle"
	}
	return fmt.Sprintf("%s-%d.rdp%s", prefix, nodeID, suffix)
}
