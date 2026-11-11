package rdp

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/tailcfg"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/client"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/vip"
	pb "github.com/open-beagle/awecloud-signaling-desktop/pkg/proto"
)

type fakeWhoIsClient struct {
	mu        sync.RWMutex
	responses map[string]*apitype.WhoIsResponse
}

func (f *fakeWhoIsClient) WhoIs(_ context.Context, remoteAddr string) (*apitype.WhoIsResponse, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if resp, ok := f.responses[remoteAddr]; ok {
		return resp, nil
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		if resp, ok := f.responses[host]; ok {
			return resp, nil
		}
	}
	return nil, fmt.Errorf("peer not found: %s", remoteAddr)
}

func TestDetectorParsersZeroHandshake(t *testing.T) {
	// 3389 = 0x0D3D -> Win32 MIB_TCPROW 网络字节序低 16 位存储为 0x3D0D (15629)
	require.Equal(t, 3389, parseWin32NetworkPort(0x00003D0D))

	sampleProcNetTCP := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:0016 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0
   1: 0100007F:0D3D 0100007F:C350 01 00000000:00000000 00:00000000 00000000     0        0 12346 1 0000000000000000 20 4 30 10 -1
   2: 00000000:0D3D 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12347 1 0000000000000000 100 0 0 10 0
`
	require.True(t, hasListeningPortInProcNetTCP(sampleProcNetTCP, 3389))
	require.False(t, hasListeningPortInProcNetTCP(sampleProcNetTCP, 13389))
}

func TestInboundGateEnforcesNodeWhoIsAndForceClosesOnRevocation(t *testing.T) {
	// 启动一个模拟本机 3389 回显服务
	localBackend, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer localBackend.Close()
	backendPort := localBackend.Addr().(*net.TCPAddr).Port

	go func() {
		for {
			conn, acceptErr := localBackend.Accept()
			if acceptErr != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}(conn)
		}
	}()

	now := time.Now()
	whois := &fakeWhoIsClient{
		responses: map[string]*apitype.WhoIsResponse{
			// 合法同账号 Desktop 节点（HeadscaleNodeID=502）
			"100.64.0.2": {
				Node:        &tailcfg.Node{ID: 502, Name: "travel-mac"},
				UserProfile: &tailcfg.UserProfile{LoginName: "client-mengsk"},
			},
			// 同账号下的 CloudIDE 非 Desktop 节点（HeadscaleNodeID=503，不在白名单中）
			"100.64.0.3": {
				Node:        &tailcfg.Node{ID: 503, Name: "cloudide-pod"},
				UserProfile: &tailcfg.UserProfile{LoginName: "client-mengsk"},
			},
			// 跨账号临时授权同事 Desktop 节点（HeadscaleNodeID=601）
			"100.64.0.20": {
				Node:        &tailcfg.Node{ID: 601, Name: "bob-laptop"},
				UserProfile: &tailcfg.UserProfile{LoginName: "client-bob@headscale.local"},
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
		LocalPort:      backendPort,
	})
	gate.ApplyPolicy(&pb.DesktopInboundRDPPolicy{
		EffectiveEnabled: true,
		RdpMode:          "auto",
		TargetPort:       int32(backendPort),
		IssuedAtUnix:     now.Unix(),
		ValidUntilUnix:   now.Add(5 * time.Minute).Unix(),
		AllowedPeers: []*pb.AllowedRDPPeer{
			{
				HeadscaleNodeId:   502,
				SourceNodeId:      12,
				SourceUserName:    "mengsk",
				SourceDeviceName:  "travel-mac",
				HeadscaleUserName: "client-mengsk",
			},
			{
				HeadscaleNodeId:   601,
				SourceNodeId:      25,
				SourceUserName:    "bob",
				SourceDeviceName:  "bob-laptop",
				GrantId:           99,
				ExpiresAtUnix:     now.Add(10 * time.Minute).Unix(),
				HeadscaleUserName: "client-bob",
			},
		},
	})

	require.True(t, gate.IsCurrentlyExposed())

	// 1. 非 3389 端口不拦截
	_, intercept := gate.FallbackTCPHandler(netip.MustParseAddrPort("100.64.0.2:50000"), netip.MustParseAddrPort("100.64.0.1:22"))
	require.False(t, intercept)

	// 2. 同账号 CloudIDE 节点 (503) 访问 3389 -> 必须被节点级门禁拒绝
	handlerCloudIDE, intercept := gate.FallbackTCPHandler(netip.MustParseAddrPort("100.64.0.3:50001"), netip.MustParseAddrPort("100.64.0.1:3389"))
	require.True(t, intercept)
	ideClient, ideServer := net.Pipe()
	go handlerCloudIDE(ideServer)
	_ = ideClient.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 8)
	_, readErr := ideClient.Read(buf)
	require.Error(t, readErr)
	_ = ideClient.Close()

	// 3. 跨账号已授权节点 (601) 访问 3389 -> 建立双向会话，随后在策略撤销时立即被强制掐断
	handlerBob, intercept := gate.FallbackTCPHandler(netip.MustParseAddrPort("100.64.0.20:50002"), netip.MustParseAddrPort("100.64.0.1:3389"))
	require.True(t, intercept)
	bobClient, bobServer := net.Pipe()
	doneCh := make(chan struct{})
	go func() {
		handlerBob(bobServer)
		close(doneCh)
	}()

	_, err = bobClient.Write([]byte("ping"))
	require.NoError(t, err)
	echo := make([]byte, 4)
	_, err = io.ReadFull(bobClient, echo)
	require.NoError(t, err)
	require.Equal(t, "ping", string(echo))
	require.Equal(t, 1, gate.ActiveSessionCount())

	// 下发移除 bob (GrantId=99) 的新策略 -> 存量 TCP 连接必须立即被强制关闭
	gate.ApplyPolicy(&pb.DesktopInboundRDPPolicy{
		EffectiveEnabled: true,
		RdpMode:          "auto",
		TargetPort:       int32(backendPort),
		IssuedAtUnix:     now.Unix(),
		ValidUntilUnix:   now.Add(5 * time.Minute).Unix(),
		AllowedPeers: []*pb.AllowedRDPPeer{
			{
				HeadscaleNodeId:   502,
				SourceNodeId:      12,
				SourceUserName:    "mengsk",
				SourceDeviceName:  "travel-mac",
				HeadscaleUserName: "client-mengsk",
			},
		},
	})

	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("expected active RDP session to be force-closed immediately upon grant revocation")
	}
	require.Equal(t, 0, gate.ActiveSessionCount())
	_ = bobClient.Close()

	// 等待异步审计回调完成并验证记录了 rejected / started / ended(policy_revoked)
	require.Eventually(t, func() bool {
		auditMu.Lock()
		defer auditMu.Unlock()
		var hasRejected, hasStarted, hasRevokedEnd bool
		for _, a := range audits {
			if a.EventType == "rejected" {
				hasRejected = true
			}
			if a.EventType == "started" && a.GrantID == 99 {
				hasStarted = true
			}
			if a.EventType == "ended" && a.GrantID == 99 && a.CloseReason == "policy_revoked" {
				hasRevokedEnd = true
			}
		}
		return hasRejected && hasStarted && hasRevokedEnd
	}, 2*time.Second, 20*time.Millisecond)
}

func TestOutboundManagerConflictFreePortFallbackAndForwarding(t *testing.T) {
	// 模拟远端被控机的 3389 服务
	remoteBackend, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer remoteBackend.Close()

	go func() {
		for {
			c, err := remoteBackend.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_, _ = conn.Write([]byte("RDP-READY"))
			}(c)
		}
	}()

	// 故意先占住一个首选端口，验证冲突自动避让并持久化新端口
	occupiedLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer occupiedLn.Close()
	occupiedPort := occupiedLn.Addr().(*net.TCPAddr).Port

	savedPorts := map[uint64]int{77: occupiedPort}
	var savedMu sync.Mutex

	now := time.Now()
	var nowMu sync.Mutex

	vipAlloc := vip.NewAllocator()
	mgr := NewOutboundManager(OutboundManagerOptions{
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			require.Equal(t, "100.64.0.77:3389", addr)
			var d net.Dialer
			return d.DialContext(ctx, "tcp", remoteBackend.Addr().String())
		},
		VIPAllocator: vipAlloc,
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
		IdleTTL: 10 * time.Minute,
		NowFunc: func() time.Time {
			nowMu.Lock()
			defer nowMu.Unlock()
			return now
		},
	})
	defer mgr.StopAll()

	ep, err := mgr.EnsureEndpoint(77, "Home Win 11", "100.64.0.77")
	require.NoError(t, err)
	require.Equal(t, "home-win-11-77.rdp.beagle", ep.Domain)
	require.NotEqual(t, occupiedPort, ep.LocalPort)

	savedMu.Lock()
	persistedPort := savedPorts[77]
	savedMu.Unlock()
	require.Equal(t, ep.LocalPort, persistedPort)

	// 通过分配的本地无冲突端口连接并读取远端响应
	conn, err := net.DialTimeout("tcp", ep.LocalAddr, 2*time.Second)
	require.NoError(t, err)
	banner, err := io.ReadAll(conn)
	require.NoError(t, err)
	require.Equal(t, "RDP-READY", string(banner))
	_ = conn.Close()

	// 验证空闲超时回收（等待异步转发协程完成连接计数归零）
	require.Eventually(t, func() bool {
		nowMu.Lock()
		now = now.Add(15 * time.Minute)
		nowMu.Unlock()
		return mgr.ReapIdleRoutes() == 1
	}, 2*time.Second, 10*time.Millisecond)
}
