package rdp

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/tailcfg"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/client"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/vip"
	pb "github.com/open-beagle/awecloud-signaling-desktop/pkg/proto"
)

// TestSelfVerify_L3_B3_PortConflictDeterministicVIPAndRefreshSelfHeal 自动化验证 L3 与 B2/B3：
//  1. 127.0.0.1:13389 被既有带外隧道占用时自动顺延至无冲突端口（如 :13390）并持久化端口偏好；
//  2. 本机 TermService 占用 :3389 时，确定性 VIP 自动绑定至 <vip>:<13390+>，且绝不漂移；
//  3. 当首次系统 DNS 探针被透明代理阻断后完成 hosts/DNS 自愈时，RefreshEndpoint 立即将 RecommendedTarget 升级为
//     三段式个人域名 <host>.<user>.<brand>:<port>（本机 :3389 空闲时为纯 <host>.<user>.<brand>），绝不退回 127.0.0.1。
func TestSelfVerify_L3_B3_PortConflictDeterministicVIPAndRefreshSelfHeal(t *testing.T) {
	remoteRDP, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer remoteRDP.Close()

	go func() {
		for {
			c, acceptErr := remoteRDP.Accept()
			if acceptErr != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_, _ = conn.Write([]byte("X224-RDP-OK"))
			}(c)
		}
	}()

	// 先探测一个空闲起始端口并占住，模拟本机 :13389 已被带外 SSH 转发占用
	occupiedLoopback, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", DefaultOutboundPortBase))
	var conflictPort int
	if err == nil {
		conflictPort = DefaultOutboundPortBase
		defer occupiedLoopback.Close()
	} else {
		fallbackOccupied, fbErr := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, fbErr)
		defer fallbackOccupied.Close()
		conflictPort = fallbackOccupied.Addr().(*net.TCPAddr).Port
	}

	domain := "home.alice.acme"
	expectedVIP := vip.DeterministicVIP(domain)
	require.NotEmpty(t, expectedVIP)

	// 占住 expectedVIP:3389，模拟 Windows 主控机自身开启了 TermService (0.0.0.0:3389 SO_EXCLUSIVEADDRUSE)
	occupiedVIP3389, vip3389Err := net.Listen("tcp", fmt.Sprintf("%s:%d", expectedVIP, DefaultRDPPort))
	if vip3389Err == nil {
		defer occupiedVIP3389.Close()
	}

	var savedMu sync.Mutex
	savedPorts := map[uint64]int{88: conflictPort}
	var dnsHealed atomic.Bool

	mgr := NewOutboundManager(OutboundManagerOptions{
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			require.Equal(t, "100.64.0.3:3389", addr)
			var d net.Dialer
			return d.DialContext(ctx, "tcp", remoteRDP.Addr().String())
		},
		VIPAllocator: vip.NewAllocator(),
		GetSavedPort: func(nodeID uint64) int {
			savedMu.Lock()
			defer savedMu.Unlock()
			return savedPorts[nodeID]
		},
		SavePort: func(nodeID uint64, port int) {
			savedMu.Lock()
			savedPorts[nodeID] = port
			savedMu.Unlock()
		},
		VerifyVIP: func(d string, gotVIP string) bool {
			return dnsHealed.Load() && d == domain && gotVIP == expectedVIP
		},
	})
	defer mgr.StopAll()

	// 第一阶段：模拟 Proxifier 拦截动态后缀、受管 hosts 尚未同步时首次创建入口
	epBeforeHeal, err := mgr.EnsureEndpointWithDomain(88, "HOME-PC", "100.64.0.3", domain)
	require.NoError(t, err)
	require.NotEqual(t, conflictPort, epBeforeHeal.LocalPort, "端口冲突必须自动顺延")
	require.Equal(t, expectedVIP, epBeforeHeal.VIP, "VIP 必须严格等于域名 FNV-1a 确定性地址")
	require.Equal(t, epBeforeHeal.LocalPort, epBeforeHeal.VIPPort, ":3389 被占用时 VIP 应绑定至无冲突顺延端口")
	require.Equal(t, epBeforeHeal.LocalAddr, epBeforeHeal.RecommendedTarget)

	savedMu.Lock()
	require.Equal(t, epBeforeHeal.LocalPort, savedPorts[88], "新顺延端口必须持久化至偏好配置")
	savedMu.Unlock()

	// 第二阶段：模拟 PrepareDeviceRDPEndpoint 完成受管 hosts / DNS 同步后调用 RefreshEndpoint
	dnsHealed.Store(true)
	epAfterHeal, ok := mgr.RefreshEndpoint(88)
	require.True(t, ok)
	require.Equal(t, fmt.Sprintf("%s:%d", domain, epAfterHeal.VIPPort), epAfterHeal.RecommendedTarget,
		"自愈刷新后 RecommendedTarget 必须升级为三段式个人域名而非 127.0.0.1")

	// 通过 VIP:VIPPort 实际发起 TCP 连接，验证转发至远端 100.64.0.3:3389 正常连通
	vipConn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", epAfterHeal.VIP, epAfterHeal.VIPPort), 2*time.Second)
	require.NoError(t, err)
	payload, err := io.ReadAll(vipConn)
	require.NoError(t, err)
	require.Equal(t, "X224-RDP-OK", string(payload))
	_ = vipConn.Close()
}

// TestSelfVerify_L5_R4_EndToEndTunnelKillSwitchModeOffAnd5mGrantRevokeUnder1s 自动化验证 L5 与 R4：
// 1. [R4] 已授权本人 Desktop 节点放行，同账号 CloudIDE/CLI 节点与未授权他人节点被精确拒绝并记录 rejected 审计；
// 2. [L5-①] 关闭被控端 rdp_mode=disabled 后活跃会话在 < 1s 内主动断开并记录 policy_disabled；
// 3. [L5-②] 5 分钟跨账号临时共享授权连通后提前撤销，活跃会话在 < 1s 内主动断开并记录 policy_revoked；
// 4. [L5-③] 平台开启 rdp_admin_disabled 紧急熔断，活跃会话在 < 1s 内主动断开并记录 admin_disabled。
func TestSelfVerify_L5_R4_EndToEndTunnelKillSwitchModeOffAnd5mGrantRevokeUnder1s(t *testing.T) {
	localTermService, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer localTermService.Close()
	localRDPPort := localTermService.Addr().(*net.TCPAddr).Port

	go func() {
		for {
			c, acceptErr := localTermService.Accept()
			if acceptErr != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}(c)
		}
	}()

	now := time.Now()
	whois := &fakeWhoIsClient{
		responses: map[string]*apitype.WhoIsResponse{
			"100.64.0.4": {
				Node:        &tailcfg.Node{ID: 502, Name: "work-desktop"},
				UserProfile: &tailcfg.UserProfile{LoginName: "client-alice"},
			},
			"100.64.0.5": {
				Node:        &tailcfg.Node{ID: 503, Name: "alice-cloudide"},
				UserProfile: &tailcfg.UserProfile{LoginName: "client-alice"},
			},
			"100.64.0.20": {
				Node:        &tailcfg.Node{ID: 601, Name: "colleague-desktop"},
				UserProfile: &tailcfg.UserProfile{LoginName: "client-bob"},
			},
			"100.64.0.99": {
				Node:        &tailcfg.Node{ID: 701, Name: "stranger-desktop"},
				UserProfile: &tailcfg.UserProfile{LoginName: "client-mallory"},
			},
		},
	}

	var auditMu sync.Mutex
	var audits []client.RDPSessionAuditEvent

	gate := NewInboundGate(InboundGateOptions{
		WhoIsProvider: func() (WhoIsClient, error) { return whois, nil },
		AuditReporter: func(evt client.RDPSessionAuditEvent) {
			auditMu.Lock()
			audits = append(audits, evt)
			auditMu.Unlock()
		},
		NowFunc: func() time.Time { return now },
	})
	defer gate.Stop()

	gate.SetTunnelConnected(true)
	gate.UpdateLocalStatus(LocalRDPStatus{
		Supported:      true,
		ServiceEnabled: true,
		PortListening:  true,
		Ready:          true,
		LocalPort:      localRDPPort,
	})

	ownerPeer := &pb.AllowedRDPPeer{
		HeadscaleNodeId:   502,
		SourceNodeId:      10,
		SourceUserName:    "alice",
		SourceDeviceName:  "work-desktop",
		HeadscaleUserName: "client-alice",
	}
	tempGrantPeer := &pb.AllowedRDPPeer{
		HeadscaleNodeId:   601,
		SourceNodeId:      20,
		SourceUserName:    "bob",
		SourceDeviceName:  "colleague-desktop",
		GrantId:           505,
		ExpiresAtUnix:     now.Add(5 * time.Minute).Unix(),
		HeadscaleUserName: "client-bob",
	}

	gate.ApplyPolicy(&pb.DesktopInboundRDPPolicy{
		EffectiveEnabled: true,
		RdpMode:          "auto",
		TargetPort:       int32(localRDPPort),
		IssuedAtUnix:     now.Unix(),
		ValidUntilUnix:   now.Add(5 * time.Minute).Unix(),
		AllowedPeers:     []*pb.AllowedRDPPeer{ownerPeer, tempGrantPeer},
	})

	dialThroughGate := func(srcIP string) (net.Conn, error) {
		src := netip.MustParseAddrPort(srcIP + ":52000")
		dst := netip.MustParseAddrPort("100.64.0.3:3389")
		handler, ok := gate.FallbackTCPHandler(src, dst)
		if !ok || handler == nil {
			return nil, fmt.Errorf("inbound gate not exposed")
		}
		cClient, cServer := net.Pipe()
		go handler(cServer)
		return cClient, nil
	}

	// [R4] 同账号 CloudIDE (503) 与未授权他人 (701) 均被拦截关闭
	for _, deniedIP := range []string{"100.64.0.5", "100.64.0.99"} {
		conn, dErr := dialThroughGate(deniedIP)
		require.NoError(t, dErr)
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		var one [1]byte
		_, rErr := conn.Read(one[:])
		require.Error(t, rErr)
		_ = conn.Close()
	}

	// 封装通过真实 OutboundManager 建立活跃会话并测量策略变更断连耗时的辅助断言
	assertDisconnectUnder1s := func(srcIP string, trigger func()) time.Duration {
		outMgr := NewOutboundManager(OutboundManagerOptions{
			Dial: func(_ context.Context, _, _ string) (net.Conn, error) {
				return dialThroughGate(srcIP)
			},
			VIPAllocator: vip.NewAllocator(),
			VerifyVIP:    func(_, _ string) bool { return true },
		})
		defer outMgr.StopAll()

		ep, epErr := outMgr.EnsureEndpointWithDomain(301, "HOME-PC", "100.64.0.3", "home.alice.acme")
		require.NoError(t, epErr)

		tcpConn, cErr := net.DialTimeout("tcp", ep.LocalAddr, 2*time.Second)
		require.NoError(t, cErr)
		defer tcpConn.Close()

		_, wErr := tcpConn.Write([]byte("RDP-FRAME"))
		require.NoError(t, wErr)
		echo := make([]byte, 9)
		_, rErr := io.ReadFull(tcpConn, echo)
		require.NoError(t, rErr)
		require.Equal(t, "RDP-FRAME", string(echo))

		start := time.Now()
		trigger()
		_ = tcpConn.SetReadDeadline(time.Now().Add(time.Second))
		var eofBuf [8]byte
		_, eofErr := tcpConn.Read(eofBuf[:])
		elapsed := time.Since(start)
		require.Error(t, eofErr, "策略变更后存量远程桌面 TCP 会话必须被主动断开")
		require.Less(t, elapsed, time.Second, "存量会话主动断开耗时必须 < 1s")
		return elapsed
	}

	// [L5-②] 5 分钟跨账号临时授权提前撤销 -> < 1s 秒断
	_ = assertDisconnectUnder1s("100.64.0.20", func() {
		gate.ApplyPolicy(&pb.DesktopInboundRDPPolicy{
			EffectiveEnabled: true,
			RdpMode:          "auto",
			TargetPort:       int32(localRDPPort),
			IssuedAtUnix:     now.Unix(),
			ValidUntilUnix:   now.Add(5 * time.Minute).Unix(),
			AllowedPeers:     []*pb.AllowedRDPPeer{ownerPeer},
		})
	})

	// [L5-①] 机主远程将 rdp_mode 设为 disabled -> < 1s 秒断
	_ = assertDisconnectUnder1s("100.64.0.4", func() {
		gate.ApplyPolicy(&pb.DesktopInboundRDPPolicy{
			EffectiveEnabled: false,
			RdpMode:          "disabled",
			TargetPort:       int32(localRDPPort),
			IssuedAtUnix:     now.Unix(),
			ValidUntilUnix:   now.Add(5 * time.Minute).Unix(),
			AllowedPeers:     nil,
		})
	})

	// 恢复机主开放后测试 [L5-③] 平台管理员开启 rdp_admin_disabled 紧急熔断 -> < 1s 秒断
	gate.ApplyPolicy(&pb.DesktopInboundRDPPolicy{
		EffectiveEnabled: true,
		RdpMode:          "auto",
		TargetPort:       int32(localRDPPort),
		IssuedAtUnix:     now.Unix(),
		ValidUntilUnix:   now.Add(5 * time.Minute).Unix(),
		AllowedPeers:     []*pb.AllowedRDPPeer{ownerPeer},
	})
	_ = assertDisconnectUnder1s("100.64.0.4", func() {
		gate.ApplyPolicy(&pb.DesktopInboundRDPPolicy{
			EffectiveEnabled: false,
			AdminDisabled:    true,
			RdpMode:          "auto",
			TargetPort:       int32(localRDPPort),
			IssuedAtUnix:     now.Unix(),
			ValidUntilUnix:   now.Add(5 * time.Minute).Unix(),
			AllowedPeers:     nil,
		})
	})
	require.False(t, gate.IsCurrentlyExposed())

	// 校验全部审计事件齐全：rejected(503 & 701) + started + ended(policy_revoked / policy_disabled / admin_disabled)
	require.Eventually(t, func() bool {
		auditMu.Lock()
		defer auditMu.Unlock()
		reasons := make(map[string]bool)
		startedCount := 0
		for _, a := range audits {
			if a.EventType == "started" {
				startedCount++
			}
			if a.CloseReason != "" {
				reasons[a.EventType+":"+a.CloseReason] = true
			}
		}
		return startedCount >= 3 &&
			reasons["rejected:peer_node_503_not_in_allowlist"] &&
			reasons["rejected:peer_node_701_not_in_allowlist"] &&
			reasons["ended:policy_revoked"] &&
			reasons["ended:policy_disabled"] &&
			reasons["ended:admin_disabled"]
	}, 2*time.Second, 20*time.Millisecond)
}

// TestSelfVerify_R5_ZeroHandshakeProbeNoAcceptNoiseAndReconnectSelfHeal 自动化验证 R5：
// 1. 内核 TCP 监听表探测在连续高频轮询下对目标监听端口产生 0 次 TCP Accept 握手噪声；
// 2. 控制面/隧道断连后自动收口暴露状态，恢复重连并重新应用策略后自动恢复 rdp_tunnel_exposed=true。
func TestSelfVerify_R5_ZeroHandshakeProbeNoAcceptNoiseAndReconnectSelfHeal(t *testing.T) {
	mock3389, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer mock3389.Close()
	mockPort := mock3389.Addr().(*net.TCPAddr).Port

	var acceptNoiseCount atomic.Int64
	go func() {
		for {
			c, acceptErr := mock3389.Accept()
			if acceptErr != nil {
				return
			}
			acceptNoiseCount.Add(1)
			_ = c.Close()
		}
	}()

	supported, listening := isPlatformTCPPortListening(mockPort)
	if supported {
		require.True(t, listening, "零握手内核监听表探测必须准确识别本地监听端口")
	}

	var probeTicks atomic.Int64
	detector := &Detector{
		interval: 15 * time.Millisecond,
		probe: func() LocalRDPStatus {
			probeTicks.Add(1)
			_, portUp := isPlatformTCPPortListening(mockPort)
			if !supported {
				portUp = true
			}
			return LocalRDPStatus{
				Supported:      true,
				ServiceEnabled: true,
				PortListening:  portUp,
				Ready:          portUp,
				LocalPort:      mockPort,
				OS:             "windows",
			}
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	detector.Start(ctx)
	require.Eventually(t, func() bool {
		return probeTicks.Load() >= 5
	}, time.Second, 10*time.Millisecond)
	cancel()

	require.Equal(t, int64(0), acceptNoiseCount.Load(), "零握手探测轮询严禁对目标端口发起任何 TCP 连接握手")

	// 验证断连收口与重连状态自愈
	now := time.Now()
	var exposureHistory []bool
	var expMu sync.Mutex

	gate := NewInboundGate(InboundGateOptions{
		OnExposureChange: func(_ bool, _ int, tunnelExposed bool) {
			expMu.Lock()
			exposureHistory = append(exposureHistory, tunnelExposed)
			expMu.Unlock()
		},
		NowFunc: func() time.Time { return now },
	})
	defer gate.Stop()

	policy := &pb.DesktopInboundRDPPolicy{
		EffectiveEnabled: true,
		RdpMode:          "auto",
		TargetPort:       int32(mockPort),
		IssuedAtUnix:     now.Unix(),
		ValidUntilUnix:   now.Add(5 * time.Minute).Unix(),
	}

	gate.SetTunnelConnected(true)
	gate.UpdateLocalStatus(detector.Current())
	gate.ApplyPolicy(policy)
	require.True(t, gate.IsCurrentlyExposed())

	// 模拟隧道/控制面断连 -> 立即收口
	gate.SetTunnelConnected(false)
	require.False(t, gate.IsCurrentlyExposed())

	// 模拟重连成功 -> 自动恢复暴露状态
	gate.SetTunnelConnected(true)
	gate.ApplyPolicy(policy)
	require.True(t, gate.IsCurrentlyExposed())

	expMu.Lock()
	require.Equal(t, []bool{true, false, true}, exposureHistory)
	expMu.Unlock()
}
