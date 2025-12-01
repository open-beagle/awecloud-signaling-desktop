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

// BUILD_URL 编译时注入的默认服务器地址
var BUILD_URL = ""

// buildNumber 构建次数（编译时注入）
var buildNumber = "0"

// App struct
type App struct {
	ctx context.Context

	// 配置
	config *config.Config

	// Desktop-Web 线程（gRPC 客户端）
	desktopClient *client.DesktopClient

	// Desktop-FRP 线程（FRP 客户端）
	desktopFRP *frp.DesktopFRP

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

	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Failed to load config: %v, using defaults", err)
		cfg = &config.Config{
			ServerAddress:   getDefaultServerAddress(),
			RememberMe:      true,
			PortPreferences: make(map[int64]int),
		}
	}

	// 服务器地址逻辑：
	// 1. 如果配置文件中有地址且不为空，使用配置文件中的地址（用户已登录过）
	// 2. 如果配置文件中没有地址，使用 BUILD_URL（初次登录）
	if cfg.ServerAddress == "" {
		cfg.ServerAddress = getDefaultServerAddress()
		log.Printf("Using default server address (first time): %s", cfg.ServerAddress)
	} else {
		log.Printf("Using saved server address: %s", cfg.ServerAddress)
	}

	a.config = cfg

	log.Printf("Desktop app started")
	log.Printf("Version: %s, Build: %s, Commit: %s", version, buildNumber, gitCommit)
}

// getDefaultServerAddress 获取默认的 Server 地址
// 优先级：BUILD_URL > 硬编码默认值
func getDefaultServerAddress() string {
	if BUILD_URL != "" {
		log.Printf("使用编译时注入的 BUILD_URL: %s", BUILD_URL)
		return BUILD_URL
	}
	return "localhost:8080"
}

// shutdown is called when the app is closing
func (a *App) shutdown(ctx context.Context) {
	if a.desktopClient != nil {
		a.desktopClient.Stop()
	}
	if a.desktopFRP != nil {
		a.desktopFRP.Stop()
	}
	log.Printf("Desktop app shutdown")
}

// Login 用户登录
func (a *App) Login(serverAddr, clientID, clientSecret string, rememberMe bool) error {
	log.Printf("[App] Login: serverAddr=%s, clientID=%s, rememberMe=%v", serverAddr, clientID, rememberMe)

	// 更新并保存配置
	a.config.ServerAddress = serverAddr
	a.config.ClientID = clientID
	a.config.RememberMe = rememberMe

	// 先保存基本信息
	if err := a.config.Save(); err != nil {
		log.Printf("Warning: failed to save config: %v", err)
	}

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
	if clientSecret == "" && a.config.HasValidToken() {
		log.Printf("Attempting token authentication...")
		authResult, err = a.desktopClient.AuthWithToken(a.config.ClientID, a.config.DeviceToken)
	} else {
		log.Printf("Attempting secret authentication...")
		authResult, err = a.desktopClient.AuthWithSecret(clientID, clientSecret, rememberMe)
	}

	if err != nil {
		a.desktopClient.Stop()
		a.desktopClient = nil
		return fmt.Errorf("authentication failed: %w", err)
	}

	// 重新加载配置（AuthWithSecret已经保存过Token了）
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Warning: failed to reload config: %v", err)
	} else {
		a.config = cfg
		log.Printf("Config reloaded: ServerAddress=%s, DeviceToken length=%d, TokenExpiresAt=%d",
			a.config.ServerAddress, len(a.config.DeviceToken), a.config.TokenExpiresAt)
	}

	log.Printf("Authentication successful: %s", authResult.Message)

	// 保存隧道配置到内存，但不立即初始化 FRP
	// FRP 将在用户实际连接服务时按需初始化
	log.Printf("Login successful, FRP will be initialized when connecting to services")

	return nil
}

// initializeFRP 初始化 FRP 客户端（按需初始化）
func (a *App) initializeFRP() error {
	if a.desktopFRP != nil {
		log.Printf("[App] FRP client already initialized")
		return nil
	}

	log.Printf("[App] Initializing FRP client...")

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
		tunnelHost = extractHost(a.config.ServerAddress)
		tunnelAddr = fmt.Sprintf("%s:%d", tunnelHost, tunnelConfig.TunnelPort)
		log.Printf("[App] Using tunnel address: %s", tunnelAddr)
	} else {
		// 使用默认端口
		tunnelHost = extractHost(a.config.ServerAddress)
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

	// 创建并启动 FRP 客户端
	a.desktopFRP = frp.NewDesktopFRP(tunnelHost, token, a.commandChan, a.statusChan)
	if err := a.desktopFRP.Start(); err != nil {
		a.desktopFRP = nil
		return fmt.Errorf("failed to start FRP client: %w", err)
	}

	log.Printf("[App] FRP client initialized successfully")
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
	if a.desktopFRP != nil {
		a.desktopFRP.Stop()
		a.desktopFRP = nil
	}

	// 清除Token，但保留服务器地址和ClientID（方便下次登录）
	if a.config != nil {
		a.config.ClearToken()
		if err := a.config.Save(); err != nil {
			log.Printf("[App] Failed to save config after logout: %v", err)
		} else {
			log.Printf("[App] Token cleared, config saved")
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

	// 为每个服务添加偏好端口（不修改 ServicePort）
	for _, service := range services {
		if preferredPort, exists := a.config.PortPreferences[service.InstanceID]; exists {
			service.PreferredPort = preferredPort
		} else {
			// 如果没有偏好端口，默认使用服务端口
			service.PreferredPort = service.ServicePort
		}
	}

	log.Printf("[App] Returning %d services", len(services))
	return services, nil
}

// ConnectService 连接服务
func (a *App) ConnectService(instanceID int64, localPort int) error {
	if a.desktopClient == nil {
		return fmt.Errorf("not logged in")
	}

	// 如果 FRP 客户端还未初始化，先初始化
	if a.desktopFRP == nil {
		log.Printf("[App] Initializing FRP client for first connection")
		if err := a.initializeFRP(); err != nil {
			return fmt.Errorf("failed to initialize FRP client: %w", err)
		}
	}

	// 连接服务
	if err := a.desktopClient.ConnectService(instanceID, localPort); err != nil {
		return err
	}

	// 保存端口偏好
	if a.config.PortPreferences == nil {
		a.config.PortPreferences = make(map[int64]int)
	}
	a.config.PortPreferences[instanceID] = localPort

	// 保存配置
	if err := a.config.Save(); err != nil {
		log.Printf("Failed to save port preference: %v", err)
	}

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
	return a.config
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
	log.Printf("[App] CheckSavedCredentials: RememberMe=%v", a.config.RememberMe)

	// 检查是否记住登录
	if !a.config.RememberMe {
		return nil
	}

	// 检查是否有基本信息
	if a.config.ServerAddress == "" || a.config.ClientID == "" {
		log.Printf("[App] Missing basic info: ServerAddress=%s, ClientID=%s", a.config.ServerAddress, a.config.ClientID)
		return nil
	}

	// 检查Token是否过期
	hasValidToken := a.config.HasValidToken()
	isExpired := a.config.IsTokenExpired()
	hasToken := hasValidToken && !isExpired

	log.Printf("[App] Token status: HasValidToken=%v, IsExpired=%v, HasToken=%v", hasValidToken, isExpired, hasToken)
	log.Printf("[App] DeviceToken length: %d, TokenExpiresAt: %d", len(a.config.DeviceToken), a.config.TokenExpiresAt)

	// 检查Server是否在线
	isOnline := client.CanConnectToServer(a.config.ServerAddress)
	log.Printf("[App] Server online: %v", isOnline)

	// 如果Token过期，清除
	if a.config.IsTokenExpired() {
		log.Printf("[App] Token expired, clearing...")
		a.config.ClearToken()
		a.config.Save()
	}

	return &SavedCredentials{
		ServerAddress: a.config.ServerAddress,
		ClientID:      a.config.ClientID,
		ClientSecret:  a.config.ClientSecret,
		RememberMe:    a.config.RememberMe,
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
	a.config.ClearToken()
	a.config.ClientID = ""
	a.config.ServerAddress = ""
	a.config.RememberMe = false
	return a.config.Save()
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
