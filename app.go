package main

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/banner"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/client"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/config"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/tailscale"
	appVersion "github.com/open-beagle/awecloud-signaling-desktop/internal/version"
	"github.com/wailsapp/wails/v3/pkg/application"
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

	hostname := fmt.Sprintf("desktop-%d", a.authResult.DesktopID)
	if err := a.tsManager.Connect(tsAuth.ControlURL, tsAuth.AuthKey, hostname); err != nil {
		a.tsManager = nil
		return fmt.Errorf("连接隧道失败: %w", err)
	}

	log.Printf("[App] Tunnel connected, IP: %s", a.tsManager.GetIP())

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

		services = append(services, &ServiceInfo{
			InstanceID:       uint(i + 1), // 临时使用索引作为ID
			InstanceName:     svc.Name,
			AgentName:        svc.AgentName,
			Description:      "",
			ServicePort:      0,
			ServiceIP:        "",
			Status:           "online",
			IsFavorite:       false,
			AgentTailscaleIP: agentIP,
			ListenPort:       listenPort,
			TargetAddr:       svc.TargetAddr,
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

	// 重新初始化
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
func (a *App) ToggleFavorite(instanceID uint, desktopID uint64) error {
	log.Printf("[App] ToggleFavorite: instanceID=%d, desktopID=%d", instanceID, desktopID)
	// TODO: 实现收藏功能，需要调用服务器API
	return nil
}

// GetDevices 获取设备列表
func (a *App) GetDevices() ([]*DeviceInfo, error) {
	log.Printf("[App] GetDevices called")
	// TODO: 实现获取设备列表功能
	return []*DeviceInfo{}, nil
}

// OfflineDevice 让设备下线
func (a *App) OfflineDevice(deviceToken string) error {
	log.Printf("[App] OfflineDevice: deviceToken=%s", deviceToken)
	// TODO: 实现设备下线功能
	return nil
}

// DeleteDevice 删除设备
func (a *App) DeleteDevice(deviceToken string) error {
	log.Printf("[App] DeleteDevice: deviceToken=%s", deviceToken)
	// TODO: 实现删除设备功能
	return nil
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
