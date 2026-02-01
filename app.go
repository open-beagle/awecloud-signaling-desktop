package main

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/banner"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/client"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/config"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/tailscale"
	appVersion "github.com/open-beagle/awecloud-signaling-desktop/internal/version"
)

// App struct
type App struct {
	desktopClient *client.DesktopClient
	tsManager     *tailscale.Manager
	authResult    *client.AuthResult
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
	if a.desktopClient != nil {
		a.desktopClient.Stop()
	}
	if a.tsManager != nil {
		a.tsManager.Disconnect()
	}
	log.Printf("Desktop app shutdown")
}

// Login 登录（首次登录或密码登录）
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

	var authResult *client.AuthResult
	var err error

	// 检查是否有保存的凭证
	if clientSecret == "" && config.GlobalConfig.HasValidToken() {
		log.Printf("Attempting authentication with saved credentials...")
		// 解析 desktop_id 和 secret
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
		log.Printf("Attempting login with client credentials...")
		authResult, err = a.desktopClient.Login(clientName, clientSecret)
	}

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

	return nil
}

func (a *App) Logout() {
	log.Printf("[App] Logout called")

	if a.desktopClient != nil {
		a.desktopClient.Stop()
		a.desktopClient = nil
	}
	if a.tsManager != nil {
		a.tsManager.Disconnect()
		a.tsManager = nil
	}

	a.authResult = nil

	// 清除配置
	if err := config.Delete(); err != nil {
		log.Printf("[App] Failed to delete config after logout: %v", err)
	} else {
		log.Printf("[App] Config deleted")
	}
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

	// 提取日志级别（如果有）
	level := "INFO"
	if strings.Contains(message, "[DEBUG]") {
		level = "DEBUG"
	} else if strings.Contains(message, "[WARN]") {
		level = "WARN"
	} else if strings.Contains(message, "[ERROR]") {
		level = "ERROR"
	}

	// 检查是否应该输出
	if !shouldLog(level) {
		return len(p), nil
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logLine := fmt.Sprintf("[%s] [%s] %s", timestamp, level, message)

	logBuffer = append(logBuffer, logLine)
	if len(logBuffer) > maxLogLines {
		logBuffer = logBuffer[len(logBuffer)-maxLogLines:]
	}

	fmt.Println(logLine)
	return len(p), nil
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
