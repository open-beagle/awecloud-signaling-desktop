package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/banner"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/client"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/config"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/dns"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/proxy"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/tailscale"
	appVersion "github.com/open-beagle/awecloud-signaling-desktop/internal/version"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/vip"
)

// App struct
type App struct {
	desktopClient *client.DesktopClient
	tsManager     *tailscale.Manager
	authResult    *client.AuthResult
	loginWindow   *application.WebviewWindow // 登录窗口引用（指针）
	loginMutex    sync.Mutex                 // 保护登录窗口的并发访问

	// ZTNA: DNS 劫持 + VIP 分配 + 本地代理
	dnsServer    *dns.Server
	vipAllocator *vip.Allocator
	proxyManager *proxy.Manager
	svcProxyMgr  *proxy.SVCProxyManager // K8S Service gRPC 代理管理器
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

func (a *App) startup() {
	log.SetOutput(&logWriter{})
	log.SetFlags(0) // 移除默认的时间戳，使用自定义格式

	// 打印启动横幅
	banner.Print()

	cfg, err := config.Load()
	if err != nil {
		log.Printf("Failed to load config: %v", err)
		return
	}

	config.GlobalConfig = cfg
	log.Printf("Using server address: %s", config.GlobalConfig.ServerAddress)

	a.setupSystemTray()
	log.Printf("System tray started")
}

func (a *App) setupSystemTray() {
	if mainApp == nil {
		log.Printf("[App] mainApp is nil, cannot setup system tray")
		return
	}

	systray := mainApp.SystemTray.New()
	systray.SetIcon(appIcon)

	menu := mainApp.NewMenu()
	menu.Add("显示窗口").OnClick(func(ctx *application.Context) {
		if mainWindow != nil {
			mainWindow.Show()
			mainWindow.Focus()
		}
	})
	menu.AddSeparator()
	menu.Add("退出").OnClick(func(ctx *application.Context) {
		a.shutdown()
		mainApp.Quit()
	})
	systray.SetMenu(menu)

	systray.OnClick(func() {
		if mainWindow != nil {
			mainWindow.Show()
			mainWindow.Focus()
		}
	})

	systray.OnRightClick(func() {
		systray.OpenMenu()
	})
}

func (a *App) shutdown() {
	// 清理 ZTNA 网络栈
	a.cleanupZTNA()

	if a.desktopClient != nil {
		a.desktopClient.Stop()
	}
	if a.tsManager != nil {
		a.tsManager.Disconnect()
	}
	log.Printf("Desktop app shutdown")
}

// cleanupZTNA 清理 ZTNA 网络栈
func (a *App) cleanupZTNA() {
	// 清理系统 DNS 配置
	if err := dns.CleanupSystemDNS(); err != nil {
		log.Printf("[App] 清理系统 DNS 失败: %v", err)
	}

	// 停止 DNS 服务器
	if a.dnsServer != nil {
		a.dnsServer.Stop()
		a.dnsServer = nil
	}

	// 停止所有代理
	if a.proxyManager != nil {
		a.proxyManager.StopAll()
		a.proxyManager = nil
	}

	// 停止所有 SVCProxy 代理
	if a.svcProxyMgr != nil {
		a.svcProxyMgr.StopAll()
		a.svcProxyMgr = nil
	}

	// 清空 VIP 分配器
	a.vipAllocator = nil
}

// Login 使用已保存的凭证自动认证
func (a *App) Login(serverAddr, clientName, clientSecret string, rememberMe bool) error {
	log.Printf("[App] Login: serverAddr=%s, clientName=%s, rememberMe=%v", serverAddr, clientName, rememberMe)

	config.GlobalConfig.ServerAddress = serverAddr
	config.GlobalConfig.ClientID = clientName
	config.GlobalConfig.RememberMe = rememberMe

	// 创建客户端
	log.Printf("[App] Creating Desktop client for: %s", serverAddr)
	a.desktopClient = client.NewDesktopClient(serverAddr)
	if err := a.desktopClient.Start(); err != nil {
		return fmt.Errorf("failed to start desktop client: %w", err)
	}
	log.Printf("[App] Desktop client started successfully")

	// 必须有保存的凭证才能使用此方法
	if !config.GlobalConfig.HasValidToken() {
		a.desktopClient.Stop()
		a.desktopClient = nil
		return fmt.Errorf("无有效凭证，请使用 Logto 登录")
	}

	log.Printf("Attempting authentication with saved credentials...")
	// 解析 desktop_id 和 secret
	parts := strings.Split(config.GlobalConfig.DeviceToken, ":")
	if len(parts) != 2 {
		a.desktopClient.Stop()
		a.desktopClient = nil
		return fmt.Errorf("invalid device token format")
	}

	var desktopID uint64
	fmt.Sscanf(parts[0], "%d", &desktopID)
	secret := parts[1]

	authResult, err := a.desktopClient.Authenticate(desktopID, secret)
	if err != nil {
		a.desktopClient.Stop()
		a.desktopClient = nil
		return fmt.Errorf("authentication failed: %w", err)
	}

	a.authResult = authResult

	log.Printf("Config: Server=%s, DesktopID=%d",
		config.GlobalConfig.ServerAddress, authResult.DesktopID)
	log.Printf("Authentication successful: %s", authResult.Message)

	// 登录成功后自动初始化隧道
	log.Printf("Initializing tunnel after login...")
	if err := a.initializeTailscale(); err != nil {
		log.Printf("Warning: Failed to initialize tunnel: %v", err)
		// 不返回错误，允许用户继续使用，后续可以重试
	} else {
		log.Printf("Tunnel initialized successfully, IP: %s", a.tsManager.GetIP())
	}

	return nil
}

func (a *App) initializeTailscale() error {
	if a.tsManager != nil && a.tsManager.IsConnected() {
		log.Printf("[App] Tunnel already connected")
		return nil
	}

	if a.authResult == nil {
		return fmt.Errorf("not authenticated")
	}

	log.Printf("[App] Initializing tunnel client...")

	// 从认证结果获取隧道配置
	tsAuth := a.desktopClient.GetTailscaleAuth(a.authResult)
	log.Printf("[App] Tunnel auth: control_url=%s", tsAuth.ControlURL)

	a.tsManager = tailscale.NewManager()

	hostname := a.authResult.DeviceName
	if hostname == "" {
		// 回退方案：如果没有设备名，使用 desktop-{ID}
		hostname = fmt.Sprintf("desktop-%d", a.authResult.DesktopID)
	}
	if err := a.tsManager.Connect(tsAuth.ControlURL, tsAuth.AuthKey, hostname); err != nil {
		a.tsManager = nil
		return fmt.Errorf("连接隧道失败: %w", err)
	}

	log.Printf("[App] Tunnel connected, IP: %s", a.tsManager.GetIP())

	// 设置隧道状态查询回调（用于心跳重连时获取最新状态）
	a.desktopClient.SetTunnelStatusCallback(func() (string, bool) {
		if a.tsManager != nil && a.tsManager.IsConnected() {
			return a.tsManager.GetIP(), true
		}
		return "", false
	})

	// 更新心跳信息
	a.desktopClient.UpdateHeartbeat(a.tsManager.GetIP(), true)

	// 初始化 ZTNA 网络栈（DNS + VIP + Proxy）
	if err := a.initializeZTNA(); err != nil {
		log.Printf("[App] Warning: ZTNA 初始化失败: %v", err)
		// 不返回错误，基础隧道已连接，ZTNA 是增强功能
	}

	return nil
}

// initializeZTNA 初始化 ZTNA 网络栈（DNS 劫持 + VIP 分配 + 本地代理）
func (a *App) initializeZTNA() error {
	log.Printf("[App] 初始化 ZTNA 网络栈...")

	// 1. 创建 VIP 分配器
	a.vipAllocator = vip.NewAllocator()

	// 2. 创建本地代理管理器（使用 tsnet Dial）
	a.proxyManager = proxy.NewManager(a.tsManager.Dial)

	// 2.5 创建 K8S Service gRPC 代理管理器
	a.svcProxyMgr = proxy.NewSVCProxyManager(a.tsManager.Dial)

	// 3. 创建并启动本地 DNS 服务器
	// 使用平台推荐端口：Windows 需要 53（NRPT 限制），macOS/Linux 用 15353
	dnsPort := dns.RecommendedPort()
	dnsAddr := fmt.Sprintf("127.0.0.2:%d", dnsPort)

	a.dnsServer = dns.NewServer(dnsAddr, a.resolveDomain)
	if err := a.dnsServer.Start(); err != nil {
		return fmt.Errorf("启动 DNS 服务器失败: %w", err)
	}

	// 4. 配置系统 DNS（将 .beagle 域名指向本地 DNS）
	if err := dns.ConfigureSystemDNS(dnsPort); err != nil {
		log.Printf("[App] Warning: 系统 DNS 配置失败: %v", err)
		// 不返回错误，用户可以手动配置
	}

	log.Printf("[App] ZTNA 网络栈已就绪（DNS=%s）", dnsAddr)
	return nil
}

// resolveDomain DNS 解析回调：查询 Server → 分配 VIP → 启动代理
func (a *App) resolveDomain(domain string) (string, bool) {
	// 先检查是否已有 VIP 映射
	if existingVIP, ok := a.vipAllocator.GetVIP(domain); ok {
		return existingVIP, true
	}

	// 查询 Server 域名解析
	if a.desktopClient == nil {
		log.Printf("[App] DNS 解析失败: 未登录")
		return "", false
	}

	result, err := a.desktopClient.ResolveDomain(domain)
	if err != nil {
		log.Printf("[App] DNS 解析失败 (%s): %v", domain, err)
		return "", false
	}

	// 分配 VIP
	vipAddr, err := a.vipAllocator.Allocate(domain)
	if err != nil {
		log.Printf("[App] VIP 分配失败 (%s): %v", domain, err)
		return "", false
	}

	// 根据域名类型选择代理方式
	if result.DomainType == "k8ssvc" {
		// K8S Service：通过 gRPC SVCProxy 代理
		svcProxyPort := result.SvcProxyPort
		if svcProxyPort == 0 {
			svcProxyPort = 50051 // 默认端口
		}
		svcTarget := proxy.SVCTarget{
			Domain:       domain,
			VIP:          vipAddr,
			Port:         result.TargetPort,
			AgentIP:      result.AgentIP,
			GRPCPort:     svcProxyPort,
			Namespace:    result.Namespace,
			ServiceName:  result.ServiceName,
			TargetPort:   result.TargetPort,
			EndpointName: result.EndpointName,
		}
		if err := a.svcProxyMgr.StartSVCProxy(svcTarget); err != nil {
			log.Printf("[App] SVCProxy 启动失败 (%s): %v", domain, err)
		} else {
			log.Printf("[App] SVCProxy 已启动: %s:%d → %s:%d (ns=%s, svc=%s)",
				vipAddr, result.TargetPort, result.AgentIP, svcProxyPort,
				result.Namespace, result.ServiceName)
		}
	} else {
		// SSH / K8SAPI / 其他：通过普通 TCP 代理
		remoteAddr := fmt.Sprintf("%s:%d", result.AgentIP, result.TargetPort)
		target := proxy.Target{
			Domain:     domain,
			VIP:        vipAddr,
			RemoteAddr: remoteAddr,
			Port:       result.TargetPort,
			TLS:        result.DomainType == "k8sapi", // K8S API 需要本地 TLS 终止，kubectl 默认 HTTPS
		}
		if err := a.proxyManager.StartProxy(target); err != nil {
			log.Printf("[App] 代理启动失败 (%s → %s): %v", domain, remoteAddr, err)
		} else {
			log.Printf("[App] 代理已启动: %s:%d → %s (domain=%s, type=%s)",
				vipAddr, result.TargetPort, remoteAddr, domain, result.DomainType)
		}
	}

	return vipAddr, true
}

func (a *App) Logout() {
	log.Printf("[App] Logout called")

	// 先调用 Server gRPC 注销（安全离场）
	// 在 goroutine 中执行，最多等 3 秒，避免阻塞前端
	if a.desktopClient != nil {
		done := make(chan struct{})
		go func() {
			a.desktopClient.Logout()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			log.Printf("[App] gRPC Logout 超时，跳过")
		}
	}

	// 清理 ZTNA 网络栈
	a.cleanupZTNA()

	// 断开隧道（也加超时保护）
	if a.tsManager != nil {
		done := make(chan struct{})
		go func() {
			a.tsManager.Disconnect()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			log.Printf("[App] Tunnel disconnect 超时，跳过")
		}
		a.tsManager = nil
	}

	// 停止 gRPC 客户端
	if a.desktopClient != nil {
		a.desktopClient.Stop()
		a.desktopClient = nil
	}

	a.authResult = nil

	// 清除认证信息（保留服务器地址）
	config.GlobalConfig.ClearToken()
	config.GlobalConfig.ClientID = ""
	config.GlobalConfig.RememberMe = false

	if err := config.GlobalConfig.Save(); err != nil {
		log.Printf("[App] Failed to save config after logout: %v", err)
	} else {
		log.Printf("[App] Config cleared and saved")
	}

	log.Printf("[App] Logout completed")
}

// ServiceInfo 服务信息（用于前端显示）
type ServiceInfo struct {
	InstanceID       uint   `json:"instance_id"`
	InstanceName     string `json:"instance_name"`
	AgentName        string `json:"agent_name"`
	Description      string `json:"description"`
	ServicePort      int    `json:"service_port"`
	ServiceIP        string `json:"service_ip"`
	PreferredPort    int    `json:"preferred_port,omitempty"`
	Status           string `json:"status,omitempty"`
	IsFavorite       bool   `json:"is_favorite"`
	AgentTailscaleIP string `json:"agent_tailscale_ip,omitempty"`
	ListenPort       int    `json:"listen_port,omitempty"`
	TargetAddr       string `json:"target_addr,omitempty"`
	ServiceID        string `json:"service_id,omitempty"` // 服务唯一标识
}

func (a *App) GetServices() ([]*ServiceInfo, error) {
	log.Printf("[App] GetServices called")

	if a.desktopClient == nil {
		log.Printf("[App] GetServices error: not logged in")
		return nil, fmt.Errorf("not logged in")
	}

	// 从客户端获取已授权服务
	authorizedServices := a.desktopClient.GetAuthorizedServices()
	log.Printf("[App] Got %d authorized services", len(authorizedServices))

	// 获取收藏列表
	favoriteIDs, err := a.desktopClient.GetFavoriteServices()
	if err != nil {
		log.Printf("[App] Failed to get favorite services: %v", err)
		favoriteIDs = []string{} // 失败时使用空列表
	}
	favoriteMap := make(map[string]bool)
	for _, id := range favoriteIDs {
		favoriteMap[id] = true
	}

	// 转换为前端格式
	services := make([]*ServiceInfo, 0, len(authorizedServices))
	for i, svc := range authorizedServices {
		// 解析 listen_addr（格式：IP:端口）
		var agentIP string
		var listenPort int
		if svc.ListenAddr != "" {
			parts := strings.Split(svc.ListenAddr, ":")
			if len(parts) == 2 {
				agentIP = parts[0]
				fmt.Sscanf(parts[1], "%d", &listenPort)
			}
		}

		// 服务 ID 就是 svc.Id
		serviceID := svc.Id
		isFavorite := favoriteMap[serviceID]

		services = append(services, &ServiceInfo{
			InstanceID:       uint(i + 1), // 临时使用索引作为ID
			InstanceName:     svc.Name,
			AgentName:        svc.AgentName,
			Description:      "",
			ServicePort:      0,
			ServiceIP:        "",
			Status:           "online",
			IsFavorite:       isFavorite,
			AgentTailscaleIP: agentIP,
			ListenPort:       listenPort,
			TargetAddr:       svc.TargetAddr,
			ServiceID:        serviceID, // 添加服务 ID 字段
		})
	}

	log.Printf("[App] Returning %d services", len(services))
	return services, nil
}

func (a *App) IsAuthenticated() bool {
	return a.desktopClient != nil && a.desktopClient.IsAuthenticated()
}

func (a *App) GetConfig() *config.Config {
	return config.GlobalConfig
}

type VersionInfo struct {
	Version     string `json:"version"`
	GitCommit   string `json:"gitCommit"`
	BuildDate   string `json:"buildDate"`
	BuildNumber string `json:"buildNumber"`
}

func (a *App) GetVersion() *VersionInfo {
	return &VersionInfo{
		Version:     appVersion.Version,
		GitCommit:   appVersion.GitCommit,
		BuildDate:   appVersion.BuildTime,
		BuildNumber: appVersion.BuildNumber,
	}
}

func (a *App) GetWindowTitle() string {
	if appVersion.BuildNumber != "0" && appVersion.BuildNumber != "" {
		// 格式：Signaling v0.2.0 (Build 46 @ 2026-01-15_02:30:39)
		return fmt.Sprintf("Signaling  %s (Build %s @ %s)", appVersion.Version, appVersion.BuildNumber, appVersion.BuildTime)
	}
	return fmt.Sprintf("Signaling  %s", appVersion.Version)
}

type SavedCredentials struct {
	ServerAddress string `json:"server_address"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
	RememberMe    bool   `json:"remember_me"`
	HasToken      bool   `json:"has_token"`
}

func (a *App) CheckSavedCredentials() *SavedCredentials {
	log.Printf("[App] CheckSavedCredentials: HasToken=%v, DeviceToken=%s",
		config.GlobalConfig.HasValidToken(),
		maskToken(config.GlobalConfig.DeviceToken))

	serverAddr := config.GlobalConfig.ServerAddress

	if config.GlobalConfig.ClientID == "" {
		return &SavedCredentials{
			ServerAddress: serverAddr,
			ClientID:      "",
			ClientSecret:  "",
			RememberMe:    true,
			HasToken:      false,
		}
	}

	hasToken := config.GlobalConfig.HasValidToken()
	log.Printf("[App] Token status: HasToken=%v, ClientID=%s", hasToken, config.GlobalConfig.ClientID)

	return &SavedCredentials{
		ServerAddress: serverAddr,
		ClientID:      config.GlobalConfig.ClientID,
		ClientSecret:  "",
		RememberMe:    true,
		HasToken:      hasToken,
	}
}

// maskToken 隐藏 token 中间部分，用于日志
func maskToken(token string) string {
	if token == "" {
		return "<empty>"
	}
	if len(token) <= 10 {
		return "***"
	}
	return token[:5] + "***" + token[len(token)-5:]
}

func (a *App) ClearCredentials() error {
	config.GlobalConfig.ClearToken()
	config.GlobalConfig.ClientID = ""
	config.GlobalConfig.ServerAddress = ""
	return config.GlobalConfig.Save()
}

func (a *App) GetLogs() []string {
	return GetRecentLogs(1000)
}

// SetLogLevel 设置日志级别
func (a *App) SetLogLevel(level string) {
	SetLogLevel(level)
	log.Printf("[INFO] Log level changed to: %s", level)
}

// GetLogLevel 获取当前日志级别
func (a *App) GetLogLevel() string {
	return GetLogLevel()
}

func (a *App) HideToTray() {
	log.Printf("[App] HideToTray called")
	if mainWindow != nil {
		mainWindow.Hide()
	}
}

// TunnelStatus 隧道状态信息
type TunnelStatus struct {
	Connected bool   `json:"connected"`
	IP        string `json:"ip"`
	Hostname  string `json:"hostname"`
	Error     string `json:"error"`
}

// GRPCStatus gRPC 连接状态
type GRPCStatus struct {
	Connected     bool   `json:"connected"`
	ServerAddress string `json:"server_address"`
	Error         string `json:"error"`
}

// GetGRPCStatus 获取 gRPC 连接状态
func (a *App) GetGRPCStatus() *GRPCStatus {
	status := &GRPCStatus{
		Connected:     false,
		ServerAddress: config.GlobalConfig.ServerAddress,
	}

	if a.desktopClient == nil {
		status.Error = "未连接"
		return status
	}

	// 检查认证状态和 gRPC 连接状态
	if !a.desktopClient.IsAuthenticated() {
		status.Error = "未认证"
		return status
	}

	if !a.desktopClient.IsGRPCConnected() {
		status.Error = "gRPC 连接断开"
		return status
	}

	status.Connected = true
	return status
}

// GetTunnelStatus 获取隧道连接状态
func (a *App) GetTunnelStatus() *TunnelStatus {
	status := &TunnelStatus{
		Connected: false,
		IP:        "",
	}

	if a.tsManager == nil {
		status.Error = "隧道未初始化"
		return status
	}

	status.Connected = a.tsManager.IsConnected()
	if status.Connected {
		status.IP = a.tsManager.GetIP()
		if a.authResult != nil {
			status.Hostname = fmt.Sprintf("desktop-%d", a.authResult.DesktopID)
		}
	} else {
		status.Error = "隧道未连接"
	}

	return status
}

// ProxyStatusInfo 代理连接状态信息
type ProxyStatusInfo struct {
	Domain     string `json:"domain"`      // 域名
	VIP        string `json:"vip"`         // 本地 VIP 地址
	Port       int    `json:"port"`        // 监听端口
	RemoteAddr string `json:"remote_addr"` // 远程地址
	Type       string `json:"type"`        // 类型：tcp / svc
	TLS        bool   `json:"tls"`         // 是否 TLS
}

// GetProxyStatus 获取所有本地代理连接状态
func (a *App) GetProxyStatus() []*ProxyStatusInfo {
	var result []*ProxyStatusInfo

	// TCP 代理状态
	if a.proxyManager != nil {
		for _, t := range a.proxyManager.GetStatus() {
			result = append(result, &ProxyStatusInfo{
				Domain:     t.Domain,
				VIP:        t.VIP,
				Port:       t.Port,
				RemoteAddr: t.RemoteAddr,
				Type:       "tcp",
				TLS:        t.TLS,
			})
		}
	}

	// SVCProxy 代理状态
	if a.svcProxyMgr != nil {
		for _, t := range a.svcProxyMgr.GetStatus() {
			result = append(result, &ProxyStatusInfo{
				Domain:     t.Domain,
				VIP:        t.VIP,
				Port:       t.Port,
				RemoteAddr: fmt.Sprintf("%s:%d", t.AgentIP, t.GRPCPort),
				Type:       "svc",
			})
		}
	}

	return result
}

// ReconnectTunnel 重新连接隧道
func (a *App) ReconnectTunnel() error {
	log.Printf("[App] ReconnectTunnel called")

	if a.desktopClient == nil {
		return fmt.Errorf("未登录")
	}

	// 先断开现有连接
	if a.tsManager != nil {
		a.tsManager.Disconnect()
		a.tsManager = nil
	}

	// 重新认证以获取新的 authKey
	log.Printf("[App] Re-authenticating to get new authKey...")
	var authResult *client.AuthResult
	var err error

	if config.GlobalConfig.HasValidToken() {
		// 使用保存的凭证重新认证
		parts := strings.Split(config.GlobalConfig.DeviceToken, ":")
		if len(parts) == 2 {
			var desktopID uint64
			fmt.Sscanf(parts[0], "%d", &desktopID)
			secret := parts[1]
			authResult, err = a.desktopClient.Authenticate(desktopID, secret)
		} else {
			err = fmt.Errorf("invalid device token format")
		}
	} else {
		return fmt.Errorf("无有效凭证，请重新登录")
	}

	if err != nil {
		return fmt.Errorf("重新认证失败: %w", err)
	}

	// 更新 authResult
	a.authResult = authResult

	// 重新初始化隧道
	if err := a.initializeTailscale(); err != nil {
		return fmt.Errorf("重连失败: %w", err)
	}

	log.Printf("[App] Tunnel reconnected, IP: %s", a.tsManager.GetIP())
	return nil
}

func (a *App) ShowFromTray() {
	log.Printf("[App] ShowFromTray called")
	if mainWindow != nil {
		mainWindow.Show()
		mainWindow.Focus()
	}
}

func (a *App) QuitApp() {
	log.Printf("[App] QuitApp called")
	a.shutdown()
	if mainApp != nil {
		mainApp.Quit()
	}
}

// ToggleFavorite 切换服务收藏状态
func (a *App) ToggleFavorite(serviceID string) (bool, error) {
	log.Printf("[App] ToggleFavorite: serviceID=%s", serviceID)

	if a.desktopClient == nil {
		return false, fmt.Errorf("未登录")
	}

	isFavorite, err := a.desktopClient.ToggleFavorite(serviceID)
	if err != nil {
		return false, err
	}

	log.Printf("[App] Service %s favorite status: %v", serviceID, isFavorite)
	return isFavorite, nil
}

// HostInfo 主机信息
type HostInfo struct {
	HostID   string   `json:"host_id"`
	HostName string   `json:"host_name"`
	TunnelIP string   `json:"tunnel_ip"`
	SSHUsers []string `json:"ssh_users"`
	Status   string   `json:"status"`
	LastSeen string   `json:"last_seen"`
}

// GetHosts 获取已授权主机列表
func (a *App) GetHosts() ([]*HostInfo, error) {
	log.Printf("[App] GetHosts called")

	if a.desktopClient == nil {
		return nil, fmt.Errorf("未登录")
	}

	clientHosts, err := a.desktopClient.GetAuthorizedHosts()
	if err != nil {
		return nil, err
	}

	// 转换为 app.HostInfo 类型
	hosts := make([]*HostInfo, 0, len(clientHosts))
	for _, h := range clientHosts {
		hosts = append(hosts, &HostInfo{
			HostID:   h.HostID,
			HostName: h.HostName,
			TunnelIP: h.TunnelIP,
			SSHUsers: h.SSHUsers,
			Status:   h.Status,
			LastSeen: h.LastSeen,
		})
	}

	log.Printf("[App] Returning %d hosts", len(hosts))
	return hosts, nil
}

// GetHostServices 获取指定主机的服务列表
func (a *App) GetHostServices(hostID string) ([]*ServiceInfo, error) {
	log.Printf("[App] GetHostServices: hostID=%s", hostID)

	if a.desktopClient == nil {
		return nil, fmt.Errorf("未登录")
	}

	services, err := a.desktopClient.GetHostServices(hostID)
	if err != nil {
		return nil, err
	}

	// 获取收藏列表
	favoriteIDs, err := a.desktopClient.GetFavoriteServices()
	if err != nil {
		log.Printf("[App] Failed to get favorite services: %v", err)
		favoriteIDs = []string{}
	}
	favoriteMap := make(map[string]bool)
	for _, id := range favoriteIDs {
		favoriteMap[id] = true
	}

	// 转换为前端格式
	result := make([]*ServiceInfo, 0, len(services))
	for i, svc := range services {
		// 解析 listen_addr（格式：IP:端口）
		var agentIP string
		var listenPort int
		if svc.ListenAddr != "" {
			parts := strings.Split(svc.ListenAddr, ":")
			if len(parts) == 2 {
				agentIP = parts[0]
				fmt.Sscanf(parts[1], "%d", &listenPort)
			}
		}

		serviceID := svc.Id
		isFavorite := favoriteMap[serviceID]

		result = append(result, &ServiceInfo{
			InstanceID:       uint(i + 1),
			InstanceName:     svc.Name,
			AgentName:        svc.AgentName,
			Description:      "",
			ServicePort:      0,
			ServiceIP:        "",
			Status:           "online", // 主机在线则服务在线
			IsFavorite:       isFavorite,
			AgentTailscaleIP: agentIP,
			ListenPort:       listenPort,
			TargetAddr:       svc.TargetAddr,
			ServiceID:        serviceID,
		})
	}

	log.Printf("[App] Returning %d services for host %s", len(result), hostID)
	return result, nil
}

// GetDevices 获取设备列表
func (a *App) GetDevices() ([]*DeviceInfo, error) {
	log.Printf("[App] GetDevices called")

	if a.desktopClient == nil {
		return nil, fmt.Errorf("未登录")
	}

	devices, err := a.desktopClient.GetMyDevices()
	if err != nil {
		return nil, err
	}

	// 转换为前端格式
	result := make([]*DeviceInfo, 0, len(devices))
	for _, d := range devices {
		result = append(result, &DeviceInfo{
			DeviceToken: d.DeviceToken,
			DeviceName:  d.DeviceName,
			OS:          d.OS,
			Arch:        d.Arch,
			Hostname:    d.Hostname,
			Status:      d.Status,
			LastUsedAt:  d.LastUsedAt,
			CreatedAt:   d.CreatedAt,
			IsCurrent:   d.IsCurrent,
			IP:          d.IP,
		})
	}

	return result, nil
}

// OfflineDevice 让设备下线
func (a *App) OfflineDevice(deviceToken string) error {
	log.Printf("[App] OfflineDevice: deviceToken=%s", deviceToken)

	if a.desktopClient == nil {
		return fmt.Errorf("未登录")
	}

	return a.desktopClient.OfflineDevice(deviceToken)
}

// DeleteDevice 删除设备
func (a *App) DeleteDevice(deviceToken string) error {
	log.Printf("[App] DeleteDevice: deviceToken=%s", deviceToken)

	if a.desktopClient == nil {
		return fmt.Errorf("未登录")
	}

	return a.desktopClient.DeleteDevice(deviceToken)
}

// GetResources 获取可访问的资源列表（SSH / K8S API / K8S Service）
func (a *App) GetResources() ([]*client.ResourceInfo, error) {
	log.Printf("[App] GetResources called")

	if a.desktopClient == nil {
		return nil, fmt.Errorf("未登录")
	}

	resources, err := a.desktopClient.GetResources()
	if err != nil {
		return nil, err
	}

	log.Printf("[App] 获取到 %d 个资源", len(resources))
	return resources, nil
}

// KubeconfigResult kubeconfig 生成结果
type KubeconfigResult struct {
	Path     string   `json:"path"`     // kubeconfig 文件路径
	Clusters []string `json:"clusters"` // 已配置的集群名称列表
	Count    int      `json:"count"`    // 集群数量
}

// clusterEntry kubeconfig 集群条目
type clusterEntry struct {
	Name   string // 集群名称（如 beijing）
	Domain string // 域名
	VIP    string // 本地 VIP 地址
	Port   int    // 端口（K8S API 默认 6443）
}

// GenerateKubeconfig 自动生成 kubeconfig，为每个已授权的 K8S API 资源创建集群条目
// 流程：获取 k8sapi 资源 → 触发 DNS 解析（分配 VIP + 启动代理）→ 生成 kubeconfig
func (a *App) GenerateKubeconfig() (*KubeconfigResult, error) {
	log.Printf("[App] GenerateKubeconfig called")

	if a.desktopClient == nil {
		return nil, fmt.Errorf("未登录")
	}

	// 1. 获取资源列表
	resources, err := a.desktopClient.GetResources()
	if err != nil {
		return nil, fmt.Errorf("获取资源列表失败: %w", err)
	}

	// 2. 筛选 k8sapi 类型资源
	var k8sResources []*client.ResourceInfo
	for _, r := range resources {
		if r.Type == "k8sapi" {
			k8sResources = append(k8sResources, r)
		}
	}

	if len(k8sResources) == 0 {
		return &KubeconfigResult{Count: 0}, nil
	}

	// 3. 对每个 k8sapi 域名触发 DNS 解析（确保 VIP 已分配、TLS 代理已启动）
	var clusters []clusterEntry

	for _, r := range k8sResources {
		// 触发 DNS 解析（会自动分配 VIP + 启动 TLS 代理）
		vipAddr, ok := a.resolveDomain(r.Domain)
		if !ok {
			log.Printf("[App] kubeconfig: 域名解析失败 %s，跳过", r.Domain)
			continue
		}

		// 从域名提取集群名称：kubernetes.{agent_name}.beagle → agent_name
		// 或 kubernetes.{endpoint}.{agent_name}.beagle → endpoint-agent_name
		clusterName := extractClusterName(r.Domain, r.AgentName)

		// K8S API 默认端口 6443
		port := 6443
		if r.Port > 0 {
			port = int(r.Port)
		}

		clusters = append(clusters, clusterEntry{
			Name:   clusterName,
			Domain: r.Domain,
			VIP:    vipAddr,
			Port:   port,
		})
	}

	if len(clusters) == 0 {
		return &KubeconfigResult{Count: 0}, nil
	}

	// 4. 生成 kubeconfig YAML
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("获取 HOME 目录失败: %w", err)
	}

	kubeDir := homeDir + "/.kube"
	if err := os.MkdirAll(kubeDir, 0700); err != nil {
		return nil, fmt.Errorf("创建 .kube 目录失败: %w", err)
	}

	kubeconfigPath := kubeDir + "/config"

	// 读取现有 kubeconfig（如果存在）
	existingContent, _ := os.ReadFile(kubeconfigPath)

	// 构建新的 kubeconfig 内容
	newContent := buildKubeconfig(string(existingContent), clusters)

	if err := os.WriteFile(kubeconfigPath, []byte(newContent), 0600); err != nil {
		return nil, fmt.Errorf("写入 kubeconfig 失败: %w", err)
	}

	clusterNames := make([]string, 0, len(clusters))
	for _, c := range clusters {
		clusterNames = append(clusterNames, c.Name)
	}

	log.Printf("[App] kubeconfig 已生成: %s（%d 个集群）", kubeconfigPath, len(clusters))
	return &KubeconfigResult{
		Path:     kubeconfigPath,
		Clusters: clusterNames,
		Count:    len(clusters),
	}, nil
}

// extractClusterName 从域名和 Agent 名称提取集群名称
// kubernetes.beijing.beagle → beijing
// kubernetes.beagle-241.beijing.beagle → beagle-241-beijing
func extractClusterName(domain, agentName string) string {
	// 移除域名后缀（.beagle 或其他）
	parts := strings.Split(domain, ".")
	if len(parts) < 3 {
		return agentName
	}

	// 去掉第一个 "kubernetes" 和最后一个后缀
	middle := parts[1 : len(parts)-1]
	return strings.Join(middle, "-")
}

// buildKubeconfig 构建 kubeconfig YAML 内容
// 使用标记块方式，避免影响用户已有配置
func buildKubeconfig(existing string, clusters []clusterEntry) string {
	// 如果没有现有配置，生成完整的 kubeconfig
	// 如果有现有配置，在标记块内替换

	marker := "# >>> AWECloud Signaling Clusters >>>"
	markerEnd := "# <<< AWECloud Signaling Clusters <<<"

	// 构建集群、上下文、用户条目
	var clusterYAML, contextYAML, userYAML strings.Builder

	for _, c := range clusters {
		// cluster 条目
		clusterYAML.WriteString("- cluster:\n")
		clusterYAML.WriteString(fmt.Sprintf("    server: https://%s:%d\n", c.VIP, c.Port))
		clusterYAML.WriteString("    insecure-skip-tls-verify: true\n")
		clusterYAML.WriteString(fmt.Sprintf("  name: %s\n", c.Name))

		// context 条目
		contextYAML.WriteString("- context:\n")
		contextYAML.WriteString(fmt.Sprintf("    cluster: %s\n", c.Name))
		contextYAML.WriteString("    user: signaling-user\n")
		contextYAML.WriteString(fmt.Sprintf("  name: %s\n", c.Name))

		// user 条目（共用一个 signaling-user）
	}

	// 只需要一个 user 条目
	userYAML.WriteString("- name: signaling-user\n")
	userYAML.WriteString("  user: {}\n")

	signalingBlock := fmt.Sprintf(`%s
apiVersion: v1
kind: Config
clusters:
%scontexts:
%susers:
%s%s`,
		marker,
		clusterYAML.String(),
		contextYAML.String(),
		userYAML.String(),
		markerEnd,
	)

	// 如果没有现有配置，直接返回完整 kubeconfig
	if strings.TrimSpace(existing) == "" {
		return fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: %s
clusters:
%scontexts:
%susers:
%s`, clusters[0].Name, clusterYAML.String(), contextYAML.String(), userYAML.String())
	}

	// 如果已有标记块，替换
	beginIdx := strings.Index(existing, marker)
	endIdx := strings.Index(existing, markerEnd)
	if beginIdx >= 0 && endIdx >= 0 {
		endIdx += len(markerEnd)
		if endIdx < len(existing) && existing[endIdx] == '\n' {
			endIdx++
		}
		return existing[:beginIdx] + signalingBlock + "\n" + existing[endIdx:]
	}

	// 没有标记块，尝试合并到现有 kubeconfig
	// 简单方案：在 clusters/contexts/users 段追加
	// 复杂合并容易出错，改用独立文件方案
	signalingConfigPath := strings.Replace(existing, "", "", 0) // no-op
	_ = signalingConfigPath

	// 写入独立文件 ~/.kube/signaling-config，提示用户设置 KUBECONFIG
	return existing + "\n" + signalingBlock + "\n"
}

// CheckVersion 检查版本更新
func (a *App) CheckVersion() (*VersionCheckResult, error) {
	log.Printf("[App] CheckVersion called")
	// TODO: 实现版本检查功能
	return &VersionCheckResult{
		HasUpdate:      false,
		LatestVersion:  appVersion.Version,
		CurrentVersion: appVersion.Version,
		UpdateURL:      "",
	}, nil
}

// DeviceInfo 设备信息
type DeviceInfo struct {
	DeviceToken string `json:"device_token"`
	DeviceName  string `json:"device_name"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	Hostname    string `json:"hostname"`
	Status      string `json:"status"`
	LastUsedAt  string `json:"last_used_at"`
	CreatedAt   string `json:"created_at"`
	IsCurrent   bool   `json:"is_current"`
	IP          string `json:"ip"`
}

// VersionCheckResult 版本检查结果
type VersionCheckResult struct {
	HasUpdate      bool   `json:"has_update"`
	LatestVersion  string `json:"latest_version"`
	CurrentVersion string `json:"current_version"`
	UpdateURL      string `json:"update_url"`
	ReleaseNotes   string `json:"release_notes"`
}

var (
	logBuffer   []string
	logMutex    sync.Mutex
	maxLogLines = 5000
	logLevel    = "INFO" // 默认日志级别：DEBUG, INFO, WARN, ERROR
)

// SetLogLevel 设置日志级别
func SetLogLevel(level string) {
	logMutex.Lock()
	defer logMutex.Unlock()
	logLevel = strings.ToUpper(level)
}

// GetLogLevel 获取当前日志级别
func GetLogLevel() string {
	logMutex.Lock()
	defer logMutex.Unlock()
	return logLevel
}

// shouldLog 判断是否应该输出日志
func shouldLog(level string) bool {
	levels := map[string]int{
		"DEBUG": 0,
		"INFO":  1,
		"WARN":  2,
		"ERROR": 3,
	}

	currentLevel := levels[logLevel]
	msgLevel := levels[strings.ToUpper(level)]

	return msgLevel >= currentLevel
}

func LogToBuffer(message string) {
	logMutex.Lock()
	defer logMutex.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logLine := fmt.Sprintf("[%s] %s", timestamp, message)

	logBuffer = append(logBuffer, logLine)
	if len(logBuffer) > maxLogLines {
		logBuffer = logBuffer[len(logBuffer)-maxLogLines:]
	}

	log.Println(message)
}

func GetRecentLogs(n int) []string {
	logMutex.Lock()
	defer logMutex.Unlock()

	if n > len(logBuffer) {
		n = len(logBuffer)
	}

	if n == 0 {
		return []string{}
	}

	return logBuffer[len(logBuffer)-n:]
}

type logWriter struct{}

func (w *logWriter) Write(p []byte) (n int, err error) {
	logMutex.Lock()
	defer logMutex.Unlock()

	message := strings.TrimSuffix(string(p), "\n")

	// 提取日志级别（如果有）并移除已有的级别标记
	level := "INFO"
	cleanMessage := message
	if strings.Contains(message, "[DEBUG]") {
		level = "DEBUG"
		cleanMessage = strings.Replace(cleanMessage, "[DEBUG] ", "", 1)
		cleanMessage = strings.Replace(cleanMessage, "[DEBUG]", "", 1)
	} else if strings.Contains(message, "[WARN]") {
		level = "WARN"
		cleanMessage = strings.Replace(cleanMessage, "[WARN] ", "", 1)
		cleanMessage = strings.Replace(cleanMessage, "[WARN]", "", 1)
	} else if strings.Contains(message, "[ERROR]") {
		level = "ERROR"
		cleanMessage = strings.Replace(cleanMessage, "[ERROR] ", "", 1)
		cleanMessage = strings.Replace(cleanMessage, "[ERROR]", "", 1)
	} else if strings.Contains(message, "[INFO]") {
		level = "INFO"
		cleanMessage = strings.Replace(cleanMessage, "[INFO] ", "", 1)
		cleanMessage = strings.Replace(cleanMessage, "[INFO]", "", 1)
	}

	// 清理消息前后空格
	cleanMessage = strings.TrimSpace(cleanMessage)

	// 检查是否应该输出
	if !shouldLog(level) {
		return len(p), nil
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logLine := fmt.Sprintf("[%s] [%s] %s", timestamp, level, cleanMessage)

	logBuffer = append(logBuffer, logLine)
	if len(logBuffer) > maxLogLines {
		logBuffer = logBuffer[len(logBuffer)-maxLogLines:]
	}

	fmt.Println(logLine)
	return len(p), nil
}

// CreateLoginSessionResult 创建登录会话结果（暴露给前端）
type CreateLoginSessionResult struct {
	SessionID string `json:"session_id"`
	LoginURL  string `json:"login_url"`
}

// CreateLoginSession 通过 gRPC 创建登录会话
// 返回 session_id 和 login_url（相对路径），前端拼接 server 地址后打开 WebView
func (a *App) CreateLoginSession(serverAddr, usernameHint string) (*CreateLoginSessionResult, error) {
	log.Printf("[App] CreateLoginSession: serverAddr=%s, usernameHint=%s", serverAddr, usernameHint)

	// 创建临时 gRPC 客户端
	tempClient := client.NewDesktopClient(serverAddr)
	if err := tempClient.Start(); err != nil {
		return nil, fmt.Errorf("连接服务器失败: %w", err)
	}
	defer tempClient.Stop()

	// 调用 gRPC CreateLoginSession
	result, err := tempClient.CreateLoginSession(usernameHint)
	if err != nil {
		return nil, fmt.Errorf("创建登录会话失败: %w", err)
	}

	log.Printf("[App] CreateLoginSession 成功: sessionID=%s, loginURL=%s", result.SessionID, result.LoginURL)

	return &CreateLoginSessionResult{
		SessionID: result.SessionID,
		LoginURL:  result.LoginURL,
	}, nil
}

// OpenBrowser 打开浏览器
func (a *App) OpenBrowser(url string) error {
	log.Printf("[App] OpenBrowser: %s", url)
	return openBrowser(url)
}

// OpenLoginWindow 打开登录窗口（在 Desktop 内部的 WebView 中）
// 用于 Logto 登录流程
func (a *App) OpenLoginWindow(loginURL string) error {
	log.Printf("[App] OpenLoginWindow: %s", loginURL)

	if mainApp == nil {
		return fmt.Errorf("mainApp is nil")
	}

	// 创建新的 WebView 窗口用于登录
	loginWindow := mainApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "登录 - Signaling Desktop",
		Width:     600,
		Height:    700,
		MinWidth:  600,
		MinHeight: 700,
		URL:       loginURL,
		BackgroundColour: application.RGBA{
			Red: 255, Green: 255, Blue: 255, Alpha: 255,
		},
	})

	// 保存窗口引用
	a.loginMutex.Lock()
	a.loginWindow = loginWindow
	a.loginMutex.Unlock()

	// 显示窗口
	loginWindow.Show()

	log.Printf("[App] Login window opened")
	return nil
}

// CloseLoginWindow 关闭登录窗口
func (a *App) CloseLoginWindow() error {
	log.Printf("[App] CloseLoginWindow")

	a.loginMutex.Lock()
	defer a.loginMutex.Unlock()

	if a.loginWindow != nil {
		a.loginWindow.Close()
		a.loginWindow = nil
		log.Printf("[App] Login window closed")
	}

	return nil
}

// ConnectService 快速连接服务
func (a *App) ConnectService(serviceID string) (string, error) {
	log.Printf("[App] ConnectService: serviceID=%s", serviceID)

	if a.desktopClient == nil {
		return "", fmt.Errorf("未登录")
	}

	// 获取服务列表，找到对应的服务
	services, err := a.GetServices()
	if err != nil {
		return "", err
	}

	var targetService *ServiceInfo
	for _, svc := range services {
		if svc.ServiceID == serviceID {
			targetService = svc
			break
		}
	}

	if targetService == nil {
		return "", fmt.Errorf("服务不存在")
	}

	// 检查服务是否在线
	if targetService.Status != "online" {
		return "", fmt.Errorf("服务离线")
	}

	// 构建连接地址
	address := fmt.Sprintf("%s:%d", targetService.AgentTailscaleIP, targetService.ListenPort)

	// 根据服务名称判断服务类型，生成连接命令
	serviceName := strings.ToLower(targetService.InstanceName)
	var command string

	if strings.Contains(serviceName, "ssh") {
		// SSH 服务
		command = fmt.Sprintf("ssh root@%s -p %d", targetService.AgentTailscaleIP, targetService.ListenPort)
	} else if strings.Contains(serviceName, "mysql") {
		// MySQL 服务
		command = fmt.Sprintf("mysql -h %s -P %d -u root -p", targetService.AgentTailscaleIP, targetService.ListenPort)
	} else if strings.Contains(serviceName, "redis") {
		// Redis 服务
		command = fmt.Sprintf("redis-cli -h %s -p %d", targetService.AgentTailscaleIP, targetService.ListenPort)
	} else if strings.Contains(serviceName, "postgres") || strings.Contains(serviceName, "pg") {
		// PostgreSQL 服务
		command = fmt.Sprintf("psql -h %s -p %d -U postgres", targetService.AgentTailscaleIP, targetService.ListenPort)
	} else if strings.Contains(serviceName, "mongo") {
		// MongoDB 服务
		command = fmt.Sprintf("mongo --host %s --port %d", targetService.AgentTailscaleIP, targetService.ListenPort)
	} else if strings.Contains(serviceName, "http") || strings.Contains(serviceName, "web") ||
		strings.Contains(serviceName, "grafana") || strings.Contains(serviceName, "kibana") {
		// HTTP/HTTPS 服务
		command = fmt.Sprintf("http://%s", address)
	} else {
		// 其他服务，返回地址
		command = address
	}

	log.Printf("[App] Generated connection command: %s", command)
	return command, nil
}

// WaitForLoginResultGRPC 通过 gRPC 双向流等待登录结果
// 此方法建立 gRPC 连接，等待 Server 推送登录结果
// 返回登录成功的凭证信息
type LoginResultGRPC struct {
	Success     bool
	Message     string
	IsDisabled  bool // 用户已禁用/待审批
	DesktopID   uint64
	DeviceToken string
	AuthKey     string
	ServerURL   string
	Username    string
}

func (a *App) WaitForLoginResultGRPC(serverAddr, sessionID, deviceFingerprint string) (*LoginResultGRPC, error) {
	log.Printf("[App] WaitForLoginResultGRPC: serverAddr=%s, sessionID=%s", serverAddr, sessionID)

	// 创建 Desktop 客户端（用于 gRPC 连接）
	desktopClient := client.NewDesktopClient(serverAddr)
	if err := desktopClient.Start(); err != nil {
		return nil, fmt.Errorf("failed to start desktop client: %w", err)
	}
	// 注意：不要在这里 defer Stop()，因为我们需要保留这个客户端用于后续的 API 调用

	// 调用 gRPC 方法 WaitForLoginResult
	// 这是一个双向流，我们发送 sessionID，Server 推送登录结果
	result, err := desktopClient.WaitForLoginResult(sessionID, deviceFingerprint)
	if err != nil {
		log.Printf("[App] WaitForLoginResult failed: %v", err)
		desktopClient.Stop()
		return nil, fmt.Errorf("wait for login result failed: %w", err)
	}

	if !result.Success {
		log.Printf("[App] Login failed: %s (isDisabled=%v)", result.Message, result.IsDisabled)
		desktopClient.Stop()

		// 用户被禁用/待审批
		if result.IsDisabled {
			return &LoginResultGRPC{
				Success:    false,
				IsDisabled: true,
				Message:    result.Message,
			}, nil
		}

		return &LoginResultGRPC{
			Success: false,
			Message: result.Message,
		}, nil
	}

	log.Printf("[App] Login successful: desktopID=%d, username=%s", result.DesktopID, result.Username)

	// 关闭登录窗口
	if err := a.CloseLoginWindow(); err != nil {
		log.Printf("[App] Failed to close login window: %v", err)
	}

	// 使用新获得的凭证在客户端上进行认证
	authResult, err := desktopClient.Authenticate(result.DesktopID, result.DeviceToken)
	if err != nil {
		log.Printf("[App] Failed to authenticate with new credentials: %v", err)
		desktopClient.Stop()
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	log.Printf("[App] Authentication successful with new credentials")

	// 保存 Desktop 客户端（重要：用于后续的 API 调用）
	a.desktopClient = desktopClient

	// 设置认证结果（重要：这样后续的 API 调用才能识别已登录状态）
	a.authResult = authResult

	// 保存凭证
	config.GlobalConfig.ServerAddress = serverAddr
	config.GlobalConfig.ClientID = result.Username
	config.GlobalConfig.DeviceToken = fmt.Sprintf("%d:%s", result.DesktopID, result.DeviceToken)
	if err := config.GlobalConfig.Save(); err != nil {
		log.Printf("[App] Failed to save config: %v", err)
	}

	// 登录成功后自动初始化隧道
	log.Printf("[App] Initializing tunnel after login...")
	if err := a.initializeTailscale(); err != nil {
		log.Printf("[App] Warning: Failed to initialize tunnel: %v", err)
		// 不返回错误，允许用户继续使用，后续可以重试
	} else {
		log.Printf("[App] Tunnel initialized successfully, IP: %s", a.tsManager.GetIP())
	}

	return &LoginResultGRPC{
		Success:     true,
		Message:     result.Message,
		DesktopID:   result.DesktopID,
		DeviceToken: result.DeviceToken,
		AuthKey:     result.AuthKey,
		ServerURL:   result.ServerURL,
		Username:    result.Username,
	}, nil
}
