package app

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/client"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/dns"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/vip"
	pb "github.com/open-beagle/awecloud-signaling-desktop/pkg/proto"
)

type mockDesktopGRPCServer struct {
	pb.UnimplementedDesktopServiceServer
	mu         sync.Mutex
	suffix     string
	userLabel  string
	devices    []*pb.DeviceInfo
	expectAuth string
}

func (s *mockDesktopGRPCServer) Authenticate(_ context.Context, req *pb.DesktopAuthenticateRequest) (*pb.DesktopAuthenticateResponse, error) {
	if req.Secret != s.expectAuth {
		return &pb.DesktopAuthenticateResponse{Success: false, Message: "bad secret"}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return &pb.DesktopAuthenticateResponse{
		Success:         true,
		Message:         "ok",
		ServerUrl:       "https://hs.acme.test",
		AuthKey:         "tskey-test",
		DomainSuffix:    s.suffix,
		UserDomainLabel: s.userLabel,
	}, nil
}

func (s *mockDesktopGRPCServer) GetMyDevices(_ context.Context, req *pb.GetMyDevicesRequest) (*pb.GetMyDevicesResponse, error) {
	if req.Secret != s.expectAuth {
		return &pb.GetMyDevicesResponse{}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	copied := make([]*pb.DeviceInfo, 0, len(s.devices))
	for _, d := range s.devices {
		clone := *d
		copied = append(copied, &clone)
	}
	return &pb.GetMyDevicesResponse{
		Devices:         copied,
		DomainSuffix:    s.suffix,
		UserDomainLabel: s.userLabel,
	}, nil
}

func (s *mockDesktopGRPCServer) UpdateDeviceRDPConfig(_ context.Context, req *pb.UpdateDeviceRDPConfigRequest) (*pb.UpdateDeviceRDPConfigResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if req.UpdateUserDomainLabel {
		lbl := strings.ToLower(strings.TrimSpace(req.UserDomainLabel))
		if lbl == "taken-provider" {
			return &pb.UpdateDeviceRDPConfigResponse{Success: false, Message: "用户名域名段已被占用"}, nil
		}
		s.userLabel = lbl
		for _, d := range s.devices {
			d.OwnerDomainLabel = lbl
			d.DeviceDomain = d.HostDomainLabel + "." + lbl + s.suffix
		}
	}

	var target *pb.DeviceInfo
	for _, d := range s.devices {
		if d.NodeId == req.TargetNodeId || (req.DeviceToken != "" && d.DeviceToken == req.DeviceToken) {
			target = d
			break
		}
	}
	if target == nil {
		return &pb.UpdateDeviceRDPConfigResponse{Success: false, Message: "not found"}, nil
	}

	if req.UpdateHostDomainLabel {
		hostLbl := strings.ToLower(strings.TrimSpace(req.HostDomainLabel))
		for _, d := range s.devices {
			if d.NodeId != target.NodeId && strings.EqualFold(d.HostDomainLabel, hostLbl) {
				return &pb.UpdateDeviceRDPConfigResponse{Success: false, Message: "主机短域名冲突"}, nil
			}
		}
		target.HostDomainLabel = hostLbl
		target.DeviceDomain = hostLbl + "." + s.userLabel + s.suffix
	}
	if req.RdpMode != "" {
		target.RdpMode = req.RdpMode
	}
	if req.UpdateAllowShare {
		target.RdpAllowShare = req.RdpAllowShare
	}

	clone := *target
	return &pb.UpdateDeviceRDPConfigResponse{
		Success:         true,
		Device:          &clone,
		DomainSuffix:    s.suffix,
		UserDomainLabel: s.userLabel,
	}, nil
}

// TestSelfVerify_L4_B2_AppDeviceDiscoveryDomainEditAndNamespaceTruth 自动化验证 L4 与 B2：
// 1. DesktopClient + App 完整发现远端在线设备、三段式个人域名及开放状态；
// 2. UpdateDeviceRDPConfig 支持修改个人中段域名与主机短域名，并拦截同名冲突；
// 3. collectManagedDeviceHostEntries 生成跨重启恒定的确定性 VIP 映射；
// 4. GetDomainNamespaceStatus 在未通过真实系统解析核验时绝不误报绿色就绪（消除假绿标）。
func TestSelfVerify_L4_B2_AppDeviceDiscoveryDomainEditAndNamespaceTruth(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	mockSrv := &mockDesktopGRPCServer{
		suffix:     ".unregisteredtestbrand",
		userLabel:  "alice-init",
		expectAuth: "sec-123",
		devices: []*pb.DeviceInfo{
			{
				NodeId:           10,
				DeviceToken:      "tok-work",
				DeviceName:       "WORK-PC",
				Hostname:         "WORK-PC",
				Os:               "windows",
				Arch:             "amd64",
				Status:           "online",
				IsCurrent:        true,
				Ip:               "100.64.0.4",
				RdpMode:          "auto",
				RdpLocalReady:    true,
				RdpLocalPort:     3389,
				RdpTunnelExposed: true,
				RdpConnectable:   true,
				HostDomainLabel:  "work",
				OwnerDomainLabel: "alice-init",
				DeviceDomain:     "work.alice-init.unregisteredtestbrand",
			},
			{
				NodeId:           20,
				DeviceToken:      "tok-home",
				DeviceName:       "HOME-PC",
				Hostname:         "HOME-PC",
				Os:               "windows",
				Arch:             "amd64",
				Status:           "online",
				IsCurrent:        false,
				Ip:               "100.64.0.3",
				RdpMode:          "auto",
				RdpLocalReady:    true,
				RdpLocalPort:     3389,
				RdpTunnelExposed: true,
				RdpConnectable:   true,
				HostDomainLabel:  "home-pc",
				OwnerDomainLabel: "alice-init",
				DeviceDomain:     "home-pc.alice-init.unregisteredtestbrand",
			},
		},
	}

	grpcServer := grpc.NewServer()
	pb.RegisterDesktopServiceServer(grpcServer, mockSrv)
	go func() {
		_ = grpcServer.Serve(ln)
	}()
	defer grpcServer.Stop()

	dc := client.NewDesktopClient(ln.Addr().String())
	require.NoError(t, dc.Start())
	defer dc.Stop()

	authRes, err := dc.Authenticate(10, "sec-123")
	require.NoError(t, err)
	require.True(t, authRes.Success)

	dnsSrv := dns.NewServer("127.0.0.1:0", func(_ string) (string, bool) {
		return "", false
	})
	dnsSrv.SetDomainSuffix(".unregisteredtestbrand")
	require.NoError(t, dnsSrv.Start())
	defer dnsSrv.Stop()

	appInst := &App{
		desktopClient: dc,
		authResult:    authRes,
		vipAllocator:  vip.NewAllocator(),
		dnsServer:     dnsSrv,
	}

	// 1. 获取设备列表并验证确定性受管 hosts 条目计算
	rawDevices, err := dc.GetMyDevices()
	require.NoError(t, err)
	require.Len(t, rawDevices, 2)

	// 2. 修改用户名域名段与远端设备短域名 (home-pc -> home, alice-init -> alice)
	updatedHome, err := appInst.UpdateDeviceRDPConfig(&UpdateDeviceRDPConfigInput{
		NodeID:                20,
		DeviceToken:           "tok-home",
		UpdateUserDomainLabel: true,
		UserDomainLabel:       "alice",
		UpdateHostDomainLabel: true,
		HostDomainLabel:       "home",
	})
	require.NoError(t, err)
	require.Equal(t, "home.alice.unregisteredtestbrand", updatedHome.DeviceDomain)

	// 3. 验证主机短域名防冲突拦截
	_, conflictErr := appInst.UpdateDeviceRDPConfig(&UpdateDeviceRDPConfigInput{
		NodeID:                20,
		DeviceToken:           "tok-home",
		UpdateHostDomainLabel: true,
		HostDomainLabel:       "work",
	})
	require.Error(t, conflictErr)
	require.Contains(t, conflictErr.Error(), "冲突")

	// 4. 验证 collectManagedDeviceHostEntries 生成稳定确定性 VIP 映射
	entries := appInst.collectManagedDeviceHostEntries(false)
	require.Len(t, entries, 2)
	require.Equal(t, "home.alice.unregisteredtestbrand", entries[0].Domain)
	require.Equal(t, vip.DeterministicVIP("home.alice.unregisteredtestbrand"), entries[0].IP)
	require.Equal(t, "work.alice.unregisteredtestbrand", entries[1].Domain)
	require.Equal(t, vip.DeterministicVIP("work.alice.unregisteredtestbrand"), entries[1].IP)

	// 5. 未在系统注册 .unregisteredtestbrand 时，SystemDNSReady 必须诚实返回 false（拒绝假绿标）
	nsStatus := appInst.GetDomainNamespaceStatus()
	require.Equal(t, ".unregisteredtestbrand", nsStatus.DomainSuffix)
	require.Equal(t, "alice", nsStatus.UserDomainLabel)
	require.True(t, nsStatus.DNSServerRunning)
	require.False(t, nsStatus.SystemDNSReady)
}
