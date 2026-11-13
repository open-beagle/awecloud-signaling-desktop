package app

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/autostart"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/client"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/config"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/dns"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/netutil"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/rdp"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/vip"
	pb "github.com/open-beagle/awecloud-signaling-desktop/pkg/proto"
)

// UpdateDeviceRDPConfigInput 前端调用修改设备远程桌面配置或个人主机/用户域名的参数
type UpdateDeviceRDPConfigInput struct {
	NodeID                uint64 `json:"node_id"`
	DeviceToken           string `json:"device_token"`
	RDPMode               string `json:"rdp_mode"`
	UpdateAllowShare      bool   `json:"update_allow_share"`
	RDPAllowShare         bool   `json:"rdp_allow_share"`
	UpdateHostDomainLabel bool   `json:"update_host_domain_label"`
	HostDomainLabel       string `json:"host_domain_label"`
	UpdateUserDomainLabel bool   `json:"update_user_domain_label"`
	UserDomainLabel       string `json:"user_domain_label"`
}

// DomainNamespaceStatus 当前客户端动态品牌域名与本机系统 DNS 拦截状态
type DomainNamespaceStatus struct {
	DomainSuffix     string `json:"domain_suffix"`
	UserDomainLabel  string `json:"user_domain_label"`
	DNSServerRunning bool   `json:"dns_server_running"`
	DNSListenAddr    string `json:"dns_listen_addr"`
	SystemDNSReady   bool   `json:"system_dns_ready"`
}

// PeerLinkState 远端设备隧道连接路径实时状态（P2P 直连或 DERP 中继）
type PeerLinkState struct {
	IP       string `json:"ip"`
	Online   bool   `json:"online"`
	Active   bool   `json:"active"`
	IsDirect bool   `json:"is_direct"`
	Relay    string `json:"relay"`
}

func clientDeviceToAppDevice(d *client.DeviceInfo) *DeviceInfo {
	if d == nil {
		return nil
	}
	return &DeviceInfo{
		DeviceToken:        d.DeviceToken,
		DeviceName:         d.DeviceName,
		OS:                 d.OS,
		Arch:               d.Arch,
		Hostname:           d.Hostname,
		Status:             d.Status,
		LastUsedAt:         d.LastUsedAt,
		CreatedAt:          d.CreatedAt,
		IsCurrent:          d.IsCurrent,
		IP:                 d.IP,
		NodeID:             d.NodeID,
		RDPMode:            d.RDPMode,
		RDPAllowShare:      d.RDPAllowShare,
		RDPAdminDisabled:   d.RDPAdminDisabled,
		RDPLocalReady:      d.RDPLocalReady,
		RDPLocalPort:       d.RDPLocalPort,
		RDPTunnelExposed:   d.RDPTunnelExposed,
		RDPConnectable:     d.RDPConnectable,
		OwnerUsername:      d.OwnerUsername,
		IsShared:           d.IsShared,
		GrantExpiresAtUnix: d.GrantExpiresAtUnix,
		HostDomainLabel:    d.HostDomainLabel,
		DeviceDomain:       d.DeviceDomain,
		OwnerDomainLabel:   d.OwnerDomainLabel,
	}
}

func rdpPortPreferenceKey(nodeID uint64) string {
	return fmt.Sprintf("rdp:%d", nodeID)
}

func (a *App) initRDPRuntimeForClient(dc *client.DesktopClient) {
	a.stopRDPRuntime()

	detectorCtx, detectorCancel := context.WithCancel(context.Background())
	a.rdpDetectorCancel = detectorCancel

	gate := rdp.NewInboundGate(rdp.InboundGateOptions{
		WhoIsProvider: func() (rdp.WhoIsClient, error) {
			if a.tsManager == nil {
				return nil, fmt.Errorf("隧道未启动")
			}
			return a.tsManager.LocalClient()
		},
		AuditReporter: func(evt client.RDPSessionAuditEvent) {
			if dc == nil {
				return
			}
			if err := dc.ReportRDPSessionAudit(evt); err != nil {
				log.Printf("[App] 上报远程桌面审计失败: %v", err)
			}
		},
		OnExposureChange: func(localReady bool, localPort int, tunnelExposed bool) {
			if dc != nil {
				dc.UpdateRDPHeartbeat(localReady, localPort, tunnelExposed)
			}
			a.emitFrontendEvent("rdp:status-updated", map[string]any{
				"local_ready":    localReady,
				"local_port":     localPort,
				"tunnel_exposed": tunnelExposed,
			})
		},
	})
	a.rdpInbound = gate

	detector := rdp.NewDetector(15*time.Second, func(st rdp.LocalRDPStatus) {
		gate.UpdateLocalStatus(st)
		if dc != nil {
			dc.UpdateRDPHeartbeat(st.Ready, st.LocalPort, gate.IsCurrentlyExposed())
		}
		a.emitFrontendEvent("rdp:local-status", st)
	})
	a.rdpDetector = detector

	initialLocal := detector.ProbeNow()
	gate.UpdateLocalStatus(initialLocal)
	if dc != nil {
		dc.SetRDPHeartbeatStatus(initialLocal.Ready, initialLocal.LocalPort, false)
		dc.SetRDPPolicyCallback(func(policy *pb.DesktopInboundRDPPolicy) {
			gate.ApplyPolicy(policy)
		})
		dc.SetDevicesUpdatedCallback(func(_ []*client.DeviceInfo) {
			a.emitFrontendEvent("devices:updated", nil)
		})
		dc.SetDomainNamespaceCallback(func(domainSuffix string, userDomainLabel string) {
			if a.dnsServer != nil && domainSuffix != "" {
				a.dnsServer.SetDomainSuffix(domainSuffix)
				_ = a.syncSystemDNSAndHosts(domainSuffix, true, false, false)
			}
			a.emitFrontendEvent("domain:updated", map[string]any{
				"domain_suffix":     domainSuffix,
				"user_domain_label": userDomainLabel,
			})
		})
	}

	detector.Start(detectorCtx)
}

func (a *App) attachRDPToTunnel() {
	if a.tsManager == nil || a.rdpInbound == nil {
		return
	}
	a.tsManager.RegisterFallbackTCPHandler(a.rdpInbound.FallbackTCPHandler)
	a.rdpInbound.SetTunnelConnected(a.tsManager.IsConnected())
	if a.desktopClient != nil {
		if cachedPolicy := a.desktopClient.GetCachedRDPPolicy(); cachedPolicy != nil {
			a.rdpInbound.ApplyPolicy(cachedPolicy)
		}
		localSt := a.GetLocalRDPStatus()
		if localSt != nil {
			a.desktopClient.UpdateRDPHeartbeat(localSt.Ready, localSt.LocalPort, a.rdpInbound.IsCurrentlyExposed())
		}
	}
}

func (a *App) initRDPOutbound() {
	if a.rdpOutbound != nil {
		a.rdpOutbound.StopAll()
		a.rdpOutbound = nil
	}
	if a.tsManager == nil {
		return
	}
	a.rdpOutbound = rdp.NewOutboundManager(rdp.OutboundManagerOptions{
		Dial:         a.tsManager.Dial,
		VIPAllocator: a.vipAllocator,
		GetSavedPort: func(nodeID uint64) int {
			a.portPreferenceMutex.RLock()
			defer a.portPreferenceMutex.RUnlock()
			if config.GlobalConfig == nil || config.GlobalConfig.PortPreferences == nil {
				return 0
			}
			return config.GlobalConfig.PortPreferences[rdpPortPreferenceKey(nodeID)]
		},
		SavePort: func(nodeID uint64, port int) {
			a.portPreferenceMutex.Lock()
			if config.GlobalConfig != nil {
				if config.GlobalConfig.PortPreferences == nil {
					config.GlobalConfig.PortPreferences = make(map[string]int)
				}
				config.GlobalConfig.PortPreferences[rdpPortPreferenceKey(nodeID)] = port
				if err := config.GlobalConfig.Save(); err != nil {
					log.Printf("[App] 保存远程桌面本地端口偏好失败: %v", err)
				}
			}
			a.portPreferenceMutex.Unlock()
		},
	})
}

func (a *App) stopRDPRuntime() {
	if a.rdpDetectorCancel != nil {
		a.rdpDetectorCancel()
		a.rdpDetectorCancel = nil
	}
	a.rdpDetector = nil

	if a.rdpInbound != nil {
		a.rdpInbound.Stop()
		a.rdpInbound = nil
	}
	if a.rdpOutbound != nil {
		a.rdpOutbound.StopAll()
		a.rdpOutbound = nil
	}
}

func (a *App) emitFrontendEvent(name string, data any) {
	if a.mainApp != nil {
		a.mainApp.Event.Emit(name, data)
	}
}

// GetLocalRDPStatus 获取本机零握手远程桌面探测状态与当前入站暴露状态
func (a *App) GetLocalRDPStatus() *rdp.LocalRDPStatus {
	if a.rdpDetector != nil {
		st := a.rdpDetector.ProbeNow()
		return &st
	}
	temp := rdp.NewDetector(15*time.Second, nil)
	st := temp.ProbeNow()
	return &st
}

// UpdateDeviceRDPConfig 修改本人名下当前或远端 Desktop 设备的远程桌面开放模式、共享许可或三段式个人域名
func (a *App) UpdateDeviceRDPConfig(input *UpdateDeviceRDPConfigInput) (*DeviceInfo, error) {
	if a.desktopClient == nil || !a.desktopClient.IsAuthenticated() {
		return nil, fmt.Errorf("未登录")
	}
	if input == nil {
		return nil, fmt.Errorf("请求参数不能为空")
	}
	updated, err := a.desktopClient.UpdateDeviceRDPConfig(client.UpdateDeviceRDPConfigParams{
		TargetNodeID:          input.NodeID,
		DeviceToken:           strings.TrimSpace(input.DeviceToken),
		RDPMode:               strings.TrimSpace(input.RDPMode),
		UpdateAllowShare:      input.UpdateAllowShare,
		RDPAllowShare:         input.RDPAllowShare,
		UpdateHostDomainLabel: input.UpdateHostDomainLabel,
		HostDomainLabel:       strings.TrimSpace(input.HostDomainLabel),
		UpdateUserDomainLabel: input.UpdateUserDomainLabel,
		UserDomainLabel:       strings.TrimSpace(input.UserDomainLabel),
	})
	if err != nil {
		return nil, err
	}
	if (input.UpdateHostDomainLabel || input.UpdateUserDomainLabel) && a.dnsServer != nil {
		_ = a.syncSystemDNSAndHosts(a.currentDomainSuffix(), true, false, false)
	}
	return clientDeviceToAppDevice(updated), nil
}

func (a *App) currentDomainSuffix() string {
	suffix := netutil.ActiveDomainSuffix()
	if a.desktopClient != nil {
		if s := a.desktopClient.GetDomainSuffix(); s != "" {
			suffix = s
		}
	}
	if suffix == "" {
		suffix = ".beagle"
	}
	return suffix
}

func (a *App) collectManagedDeviceHostEntries(fetchFresh bool) []dns.ManagedHostEntry {
	if a.desktopClient == nil || !a.desktopClient.IsAuthenticated() {
		return nil
	}
	var devices []*client.DeviceInfo
	if fetchFresh {
		if fresh, err := a.desktopClient.GetMyDevices(); err == nil {
			devices = fresh
		}
	}
	if len(devices) == 0 {
		devices = a.desktopClient.GetCachedDevices()
	}

	entries := make([]dns.ManagedHostEntry, 0, len(devices))
	for _, d := range devices {
		if d == nil {
			continue
		}
		domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(d.DeviceDomain), "."))
		if domain == "" {
			continue
		}
		targetVIP := ""
		if a.rdpOutbound != nil {
			if v, ok := a.rdpOutbound.ResolveVIP(domain); ok {
				targetVIP = v
			}
		}
		if targetVIP == "" && a.vipAllocator != nil {
			if v, ok := a.vipAllocator.GetVIP(domain); ok {
				targetVIP = v
			}
		}
		if targetVIP == "" {
			targetVIP = vip.DeterministicVIP(domain)
		}
		if targetVIP != "" {
			entries = append(entries, dns.ManagedHostEntry{
				Domain: domain,
				IP:     targetVIP,
			})
		}
	}
	return dns.SanitizeManagedHostEntries(entries)
}

func (a *App) syncSystemDNSAndHosts(suffix string, allowElevation bool, forcePrompt bool, fetchFresh bool) error {
	if a.dnsServer == nil {
		return fmt.Errorf("本地 DNS 服务尚未启动")
	}
	if strings.TrimSpace(suffix) == "" {
		suffix = a.currentDomainSuffix()
	}
	entries := a.collectManagedDeviceHostEntries(fetchFresh)
	return dns.ConfigureSystemDNSWithHosts(suffix, a.dnsServer.ListenIP(), a.dnsServer.ListenPort(), entries, allowElevation, forcePrompt)
}

// GetDomainNamespaceStatus 获取当前动态品牌域名空间与本机系统 DNS 拦截就绪状态
func (a *App) GetDomainNamespaceStatus() *DomainNamespaceStatus {
	suffix := a.currentDomainSuffix()
	userLabel := ""
	if a.desktopClient != nil {
		userLabel = a.desktopClient.GetUserDomainLabel()
	}

	dnsRunning := a.dnsServer != nil
	listenAddr := dns.RecommendedListenAddr()
	listenIP := "127.0.0.2"
	if a.dnsServer != nil {
		listenAddr = a.dnsServer.ListenAddr()
		listenIP = a.dnsServer.ListenIP()
	}

	systemReady := false
	if dnsRunning && dns.IsSystemDNSConfigured(suffix, listenIP) {
		if dns.VerifySystemDNSResolution(suffix) {
			systemReady = true
		} else if entries := a.collectManagedDeviceHostEntries(false); len(entries) > 0 && dns.AreManagedHostsConfigured(entries) {
			systemReady = true
		}
	}

	return &DomainNamespaceStatus{
		DomainSuffix:     suffix,
		UserDomainLabel:  userLabel,
		DNSServerRunning: dnsRunning,
		DNSListenAddr:    listenAddr,
		SystemDNSReady:   systemReady,
	}
}

// EnsureSystemDNSConfigured 手动触发一次性系统 DNS / NRPT 与受管确定性 hosts 持久化注册
func (a *App) EnsureSystemDNSConfigured() (*DomainNamespaceStatus, error) {
	st := a.GetDomainNamespaceStatus()
	if !st.DNSServerRunning || a.dnsServer == nil {
		return st, fmt.Errorf("本地 DNS 服务尚未启动，请确认零信任隧道已连接")
	}
	if err := a.syncSystemDNSAndHosts(st.DomainSuffix, true, true, true); err != nil {
		return a.GetDomainNamespaceStatus(), err
	}
	return a.GetDomainNamespaceStatus(), nil
}

// GetPeerLinkStates 查询在线对端节点的实时 P2P 直连 / DERP 中继路径状态
func (a *App) GetPeerLinkStates() map[string]*PeerLinkState {
	out := make(map[string]*PeerLinkState)
	if a.tsManager == nil || !a.tsManager.IsConnected() {
		return out
	}
	for ip, info := range a.tsManager.GetPeerLinkInfos() {
		out[ip] = &PeerLinkState{
			IP:       info.IP,
			Online:   info.Online,
			Active:   info.Active,
			IsDirect: info.IsDirect,
			Relay:    info.Relay,
		}
	}
	return out
}

func (a *App) findConnectableDevice(deviceToken string) (*client.DeviceInfo, error) {
	if a.desktopClient == nil || !a.desktopClient.IsAuthenticated() {
		return nil, fmt.Errorf("未登录")
	}
	deviceToken = strings.TrimSpace(deviceToken)
	if deviceToken == "" {
		return nil, fmt.Errorf("缺少目标设备标识")
	}
	devices, err := a.desktopClient.GetMyDevices()
	if err != nil {
		return nil, err
	}
	var target *client.DeviceInfo
	for _, d := range devices {
		if d != nil && d.DeviceToken == deviceToken {
			target = d
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("未找到目标设备或授权已失效")
	}
	if target.IsCurrent {
		return nil, fmt.Errorf("不能连接当前本机设备")
	}
	if target.Status != "online" || strings.TrimSpace(target.IP) == "" {
		return nil, fmt.Errorf("目标设备当前离线或隧道尚未就绪")
	}
	if target.RDPAdminDisabled {
		return nil, fmt.Errorf("目标设备已被平台管理员安全熔断")
	}
	if !target.RDPConnectable {
		return nil, fmt.Errorf("目标设备尚未就绪远程桌面（请确认已开启系统远程桌面且处于自动/开启模式）")
	}
	return target, nil
}

// PrepareDeviceRDPEndpoint 按需为目标设备创建本地无冲突代理端口并返回连接地址（供复制连接地址使用）
func (a *App) PrepareDeviceRDPEndpoint(deviceToken string) (*rdp.RDPEndpointInfo, error) {
	target, err := a.findConnectableDevice(deviceToken)
	if err != nil {
		return nil, err
	}
	if a.tsManager == nil || !a.tsManager.IsConnected() {
		return nil, fmt.Errorf("本机零信任隧道尚未连接，请稍候重试")
	}
	if a.rdpOutbound == nil {
		a.initRDPOutbound()
	}
	if a.rdpOutbound == nil {
		return nil, fmt.Errorf("本地远程桌面代理未初始化")
	}

	name := target.DeviceName
	if name == "" {
		name = target.Hostname
	}
	ep, err := a.rdpOutbound.EnsureEndpointWithDomain(target.NodeID, name, target.IP, target.DeviceDomain)
	if err != nil {
		return nil, err
	}
	if ep.Domain != "" && ep.VIP != "" && ep.RecommendedTarget == ep.LocalAddr && a.dnsServer != nil {
		_ = a.syncSystemDNSAndHosts(a.currentDomainSuffix(), true, false, false)
		if refreshed, ok := a.rdpOutbound.RefreshEndpoint(target.NodeID); ok {
			ep = refreshed
		}
	}
	return &ep, nil
}

// ConnectDeviceRDP 按需准备本地代理并一键拉起系统远程桌面客户端（Windows mstsc.exe / macOS & Linux .rdp）
func (a *App) ConnectDeviceRDP(deviceToken string) (*rdp.RDPConnectResult, error) {
	ep, err := a.PrepareDeviceRDPEndpoint(deviceToken)
	if err != nil {
		return nil, err
	}
	appDir, _ := config.GetAppDir()
	res := rdp.LaunchRDPClient(*ep, appDir)
	return &res, nil
}

// GetAutostartEnabled 查询本机当前用户是否已开启 Beagle Signal 开机登录自启动
func (a *App) GetAutostartEnabled() bool {
	return autostart.IsEnabled()
}

// SetAutostartEnabled 开启或关闭本机当前用户开机登录自启动
func (a *App) SetAutostartEnabled(enabled bool) error {
	return autostart.SetEnabled(enabled)
}
