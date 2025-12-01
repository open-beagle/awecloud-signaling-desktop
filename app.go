package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/client"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/config"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/frp"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/models"
)

// 注意：buildNumber 在 main.go 中定义，buildAddress 在 config 包中定义

// App struct
type App struct {
	ctx context.Context

	// Desktop-Web 线程（gRPC 客户端）
	desktopClient *client.DesktopClient

	// Desktop-Tunnel 线程（隧道客户端）
	desktopTunnel *frp.DesktopTunnel

	// 进程内通信通道
	commandChan chan *models.VisitorCommand
	statusChan  chan *models.VisitorStatus
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		commandChan: make(chan *models.VisitorCommand, 10),
		statusChan:  make(chan *models.VisitorStatus, 10),
	}
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// 设置日志输出到缓冲区
	log.SetOutput(&logWriter{})
	log.SetFlags(log.Ltime)

	// 加载配置（Load 已经处理了默认值）
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Failed to load config: %v", err)
		return
	}

	config.GlobalConfig = cfg // 设置全局配置
	log.Printf("Using server address: %s", config.GlobalConfig.ServerAddress)
	log.Printf("Desktop app started")
	log.Printf("Version: %s, Build: %s, Commit: %s", version, buildNumber, gitCommit)
}

// shutdown is called when the app is closing
func (a *App) shutdown(ctx context.Context) {
	if a.desktopClient != nil {
		a.desktopClient.Stop()
	}
	if a.desktopTunnel != nil {
		a.desktopTunnel.Stop()
	}
	log.Printf("Desktop app shutdown")
}

// Login 用户登录
func (a *App) Login(serverAddr, clientID, clientSecret string, rememberMe bool) error {
	log.Printf("[App] Login: serverAddr=%s, clientID=%s, rememberMe=%v", serverAddr, clientID, rememberMe)

	// 更新内存配置（不保存到文件，由 auth.go 根据 rememberMe 决定是否保存）
	config.GlobalConfig.ServerAddress = serverAddr
	config.GlobalConfig.ClientID = clientID
	config.GlobalConfig.RememberMe = rememberMe

	// 创建 Desktop-Web 线程
	log.Printf("[App] Creating Desktop-Web client for: %s", serverAddr)
	a.desktopClient = client.NewDesktopClient(serverAddr, a.commandChan, a.statusChan)
	if err := a.desktopClient.Start(); err != nil {
		return fmt.Errorf("failed to start desktop client: %w", err)
	}
	log.Printf("[App] Desktop-Web client started successfully")

	// 认证逻辑
	var authResult *client.AuthResult
	var err error

	// 如果没有提供Secret，尝试使用Token登录
	if clientSecret == "" && config.GlobalConfig.HasValidToken() {
		log.Printf("Attempting token authentication...")
		authResult, err = a.desktopClient.AuthWithToken(config.GlobalConfig.ClientID, config.GlobalConfig.DeviceToken)
	} else {
		log.Printf("Attempting secret authentication...")
		authResult, err = a.desktopClient.AuthWithSecret(clientID, clientSecret, rememberMe)
	}

	if err != nil {
		a.desktopClient.Stop()
		a.desktopClient = nil
		return fmt.Errorf("authentication failed: %w", err)
	}

	log.Printf("Config: Server=%s, Token length=%d",
		config.GlobalConfig.ServerAddress, len(config.GlobalConfig.DeviceToken))

	log.Printf("Authentication successful: %s", authResult.Message)

	// 保存隧道配置到内存，但不立即初始化隧道
	// 隧道将在用户实际连接服务时按需初始化
	log.Printf("Login successful, tunnel will be initialized when connecting to services")

	return nil
}

// initializeTunnel 初始化隧道客户端（按需初始化）
func (a *App) initializeTunnel() error {
	if a.desktopTunnel != nil {
		log.Printf("[App] Tunnel client already initialized")
		return nil
	}

	log.Printf("[App] Initializing tunnel client...")

	// 从Server获取隧道配置（不使用本地缓存）
	log.Printf("[App] Fetching tunnel config from server...")
	tunnelConfig, err := a.desktopClient.GetTunnelConfig()
	if err != nil {
		log.Printf("[App] Failed to get tunnel config: %v", err)
		return fmt.Errorf("获取隧道配置失败: %w", err)
	}

	log.Printf("[App] Tunnel config received: server=%s, port=%d, token_length=%d",
		tunnelConfig.TunnelServer, tunnelConfig.TunnelPort, len(tunnelConfig.TunnelToken))

	// 构建隧道地址
	var tunnelAddr string
	var tunnelHost string

	if tunnelConfig.TunnelServer != "" {
		// 使用完整URL
		tunnelAddr = tunnelConfig.TunnelServer
		tunnelHost = extractHost(tunnelAddr)
		log.Printf("[App] Using tunnel server URL: %s", tunnelAddr)
	} else if tunnelConfig.TunnelPort > 0 {
		// 使用端口
		tunnelHost = extractHost(config.GlobalConfig.ServerAddress)
		tunnelAddr = fmt.Sprintf("%s:%d", tunnelHost, tunnelConfig.TunnelPort)
		log.Printf("[App] Using tunnel address: %s", tunnelAddr)
	} else {
		// 使用默认端口
		tunnelHost = extractHost(config.GlobalConfig.ServerAddress)
		tunnelAddr = fmt.Sprintf("%s:7000", tunnelHost)
		log.Printf("[App] Using default tunnel address: %s", tunnelAddr)
	}

	token := tunnelConfig.TunnelToken
	if token == "" {
		log.Printf("[App] Error: Tunnel token is empty!")
		return fmt.Errorf("服务器返回的隧道Token为空")
	}

	tokenPreview := token
	if len(token) > 10 {
		tokenPreview = token[:10] + "..."
	}
	log.Printf("[App] Tunnel token: %s", tokenPreview)

	log.Printf("[App] Final tunnel config: host=%s, token_length=%d", tunnelHost, len(token))

	// 创建并启动隧道客户端
	a.desktopTunnel = frp.NewDesktopTunnel(tunnelHost, token, a.commandChan, a.statusChan)
	if err := a.desktopTunnel.Start(); err != nil {
		a.desktopTunnel = nil
		return fmt.Errorf("failed to start tunnel client: %w", err)
	}

	log.Printf("[App] Tunnel client initialized successfully")
	return nil
}

// extractHost 从地址中提取主机名
func extractHost(serverAddr string) string {
	// 移除协议前缀
	addr := serverAddr
	addr = strings.TrimPrefix(addr, "wss://")
	addr = strings.TrimPrefix(addr, "ws://")
	addr = strings.TrimPrefix(addr, "https://")
	addr = strings.TrimPrefix(addr, "http://")

	// 移除路径部分
	if idx := strings.Index(addr, "/"); idx != -1 {
		addr = addr[:idx]
	}

	// 移除端口（如果有）
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		// 确保不是 IPv6 地址
		if !strings.Contains(addr, "[") {
			addr = addr[:idx]
		}
	}

	return addr
}

// Logout 用户登出
func (a *App) Logout() {
	log.Printf("[App] Logout called")

	// 停止客户端连接
	if a.desktopClient != nil {
		a.desktopClient.Stop()
		a.desktopClient = nil
	}
	if a.desktopTunnel != nil {
		a.desktopTunnel.Stop()
		a.desktopTunnel = nil
	}

	// 根据 RememberMe 决定是保留配置还是删除配置
	if config.GlobalConfig.RememberMe {
		// 保留配置，只清除 Token
		config.GlobalConfig.ClearToken()
		if err := config.GlobalConfig.Save(); err != nil {
			log.Printf("[App] Failed to save config after logout: %v", err)
		} else {
			log.Printf("[App] Token cleared, config saved")
		}
	} else {
		// 删除配置文件
		if err := config.Delete(); err != nil {
			log.Printf("[App] Failed to delete config after logout: %v", err)
		} else {
			log.Printf("[App] Config deleted")
		}
	}
}

// GetServices 获取服务列表
func (a *App) GetServices() ([]*models.ServiceInfo, error) {
	log.Printf("[App] GetServices called")

	if a.desktopClient == nil {
		log.Printf("[App] GetServices error: not logged in")
		return nil, fmt.Errorf("not logged in")
	}

	log.Printf("[App] Calling desktopClient.GetServices()")
	services, err := a.desktopClient.GetServices()
	if err != nil {
		log.Printf("[App] GetServices error: %v", err)
		return nil, err
	}

	log.Printf("[App] Got %d services from desktopClient", len(services))

	// 端口偏好由服务器管理，PreferredPort 由服务器返回
	log.Printf("[App] Returning %d services", len(services))
	return services, nil
}

// ConnectService 连接服务
func (a *App) ConnectService(instanceID int64, localPort int) error {
	if a.desktopClient == nil {
		return fmt.Errorf("not logged in")
	}

	// 如果隧道客户端还未初始化，先初始化
	if a.desktopTunnel == nil {
		log.Printf("[App] Initializing tunnel client for first connection")
		if err := a.initializeTunnel(); err != nil {
			return fmt.Errorf("failed to initialize tunnel client: %w", err)
		}
	}

	// 连接服务
	if err := a.desktopClient.ConnectService(instanceID, localPort); err != nil {
		return err
	}

	// 端口偏好由服务器管理，不再保存到本地配置
	log.Printf("[App] Connected to service %d on port %d", instanceID, localPort)

	return nil
}

// DisconnectService 断开服务
func (a *App) DisconnectService(instanceID int64) error {
	if a.desktopClient == nil {
		return fmt.Errorf("not logged in")
	}
	return a.desktopClient.DisconnectService(instanceID)
}

// IsAuthenticated 检查是否已认证
func (a *App) IsAuthenticated() bool {
	return a.desktopClient != nil && a.desktopClient.IsAuthenticated()
}

// GetConfig 获取配置
func (a *App) GetConfig() *config.Config {
	return config.GlobalConfig
}

// VersionInfo 版本信息结构
type VersionInfo struct {
	Version     string `json:"version"`
	GitCommit   string `json:"gitCommit"`
	BuildDate   string `json:"buildDate"`
	BuildNumber string `json:"buildNumber"`
}

// GetVersion 获取版本信息
func (a *App) GetVersion() *VersionInfo {
	return &VersionInfo{
		Version:     version,
		GitCommit:   gitCommit,
		BuildDate:   buildDate,
		BuildNumber: buildNumber,
	}
}

// GetWindowTitle 获取窗口标题
func (a *App) GetWindowTitle() string {
	if buildNumber != "0" && buildNumber != "" {
		return fmt.Sprintf("awecloud-signaling  %s (Build %s)", version, buildNumber)
	}
	return fmt.Sprintf("awecloud-signaling  %s", version)
}

// CheckSavedCredentials 检查是否有保存的凭据
func (a *App) CheckSavedCredentials() *SavedCredentials {
	log.Printf("[App] CheckSavedCredentials: HasToken=%v", config.GlobalConfig.HasValidToken())

	serverAddr := config.GlobalConfig.ServerAddress

	// 如果没有配置（ClientID 为空），返回默认状态
	if config.GlobalConfig.ClientID == "" {
		return &SavedCredentials{
			ServerAddress: serverAddr,
			ClientID:      "",
			ClientSecret:  "",
			RememberMe:    true, // 默认勾选"记住我"
			HasToken:      false,
			IsOnline:      false,
		}
	}

	// 有配置，检查 Token 是否存在
	hasToken := config.GlobalConfig.HasValidToken()

	log.Printf("[App] Token status: HasToken=%v", hasToken)
	log.Printf("[App] Token length: %d", len(config.GlobalConfig.DeviceToken))

	// 检查Server是否在线
	isOnline := client.CanConnectToServer(config.GlobalConfig.ServerAddress)
	log.Printf("[App] Server online: %v", isOnline)

	return &SavedCredentials{
		ServerAddress: serverAddr,
		ClientID:      config.GlobalConfig.ClientID,
		ClientSecret:  "",
		RememberMe:    true, // 有配置说明上次勾选了"记住我"
		HasToken:      hasToken,
		IsOnline:      isOnline,
	}
}

// SavedCredentials 保存的凭据
type SavedCredentials struct {
	ServerAddress string `json:"server_address"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
	RememberMe    bool   `json:"remember_me"`
	HasToken      bool   `json:"has_token"`
	IsOnline      bool   `json:"is_online"`
}

// ClearCredentials 清除保存的凭据
func (a *App) ClearCredentials() error {
	config.GlobalConfig.ClearToken()
	config.GlobalConfig.ClientID = ""
	config.GlobalConfig.ServerAddress = ""
	return config.GlobalConfig.Save()
}

// GetLogs 获取日志（最近 100 行）
func (a *App) GetLogs() []string {
	return GetRecentLogs(100)
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

// GetDevices 获取已登录的设备列表
func (a *App) GetDevices() ([]*DeviceInfo, error) {
	if a.desktopClient == nil {
		return nil, fmt.Errorf("not logged in")
	}

	// 调用Desktop Client的GetDevices方法
	clientDevices, err := a.desktopClient.GetDevices()
	if err != nil {
		return nil, err
	}

	// 转换为App层的DeviceInfo类型
	devices := make([]*DeviceInfo, 0, len(clientDevices))
	for _, d := range clientDevices {
		devices = append(devices, &DeviceInfo{
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

	return devices, nil
}

// OfflineDevice 让设备下线
func (a *App) OfflineDevice(deviceToken string) error {
	if a.desktopClient == nil {
		return fmt.Errorf("not logged in")
	}

	log.Printf("Offline device: %s", deviceToken)
	return a.desktopClient.OfflineDevice(deviceToken)
}

// DeleteDevice 删除设备记录
func (a *App) DeleteDevice(deviceToken string) error {
	if a.desktopClient == nil {
		return fmt.Errorf("not logged in")
	}

	log.Printf("Delete device: %s", deviceToken)
	return a.desktopClient.DeleteDevice(deviceToken)
}

// 日志缓冲区
var (
	logBuffer   []string
	logMutex    sync.Mutex
	maxLogLines = 5000 // 增加到5000行
)

// LogToBuffer 添加日志到缓冲区
func LogToBuffer(message string) {
	logMutex.Lock()
	defer logMutex.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logLine := fmt.Sprintf("[%s] %s", timestamp, message)

	logBuffer = append(logBuffer, logLine)
	if len(logBuffer) > maxLogLines {
		logBuffer = logBuffer[len(logBuffer)-maxLogLines:]
	}

	// 同时输出到标准日志
	log.Println(message)
}

// GetRecentLogs 获取最近的日志
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

// logWriter 实现 io.Writer 接口，将日志写入缓冲区
type logWriter struct{}

func (w *logWriter) Write(p []byte) (n int, err error) {
	logMutex.Lock()
	defer logMutex.Unlock()

	message := string(p)
	// 移除末尾的换行符
	message = strings.TrimSuffix(message, "\n")

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logLine := fmt.Sprintf("[%s] %s", timestamp, message)

	logBuffer = append(logBuffer, logLine)
	if len(logBuffer) > maxLogLines {
		logBuffer = logBuffer[len(logBuffer)-maxLogLines:]
	}

	// 同时输出到标准输出
	fmt.Println(logLine)

	return len(p), nil
}
