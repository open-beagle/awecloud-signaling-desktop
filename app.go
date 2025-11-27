package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"awecloud-desktop/internal/client"
	"awecloud-desktop/internal/config"
	"awecloud-desktop/internal/frp"
	"awecloud-desktop/internal/models"
)

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

	// 如果配置中没有 Server 地址，使用默认值
	if cfg.ServerAddress == "" {
		cfg.ServerAddress = getDefaultServerAddress()
	}

	a.config = cfg

	log.Printf("Desktop app started")
	log.Printf("Server address: %s", a.config.ServerAddress)
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
	// 更新并保存配置
	a.config.ServerAddress = serverAddr
	a.config.ClientID = clientID
	a.config.RememberMe = rememberMe

	// 先保存基本信息
	if err := a.config.Save(); err != nil {
		log.Printf("Warning: failed to save config: %v", err)
	}

	// 创建 Desktop-Web 线程
	a.desktopClient = client.NewDesktopClient(serverAddr, a.commandChan, a.statusChan)
	if err := a.desktopClient.Start(); err != nil {
		return fmt.Errorf("failed to start desktop client: %w", err)
	}

	// 认证逻辑
	var authResult *client.AuthResult
	var err error

	// 如果没有提供Secret，尝试使用Token登录
	if clientSecret == "" && a.config.HasValidToken() {
		log.Printf("Attempting token authentication...")
		authResult, err = a.desktopClient.AuthWithToken(a.config.DeviceToken)
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

	// 创建 Desktop-FRP 线程
	// 使用从Server获取的隧道配置
	var tunnelAddr string

	if authResult.TunnelServer != "" {
		// Server 配置了公网 URL，直接使用
		tunnelAddr = authResult.TunnelServer
		log.Printf("使用 Server 提供的隧道地址: %s", tunnelAddr)
	} else {
		// Server 没有配置公网 URL，使用 Server 地址 + 端口
		tunnelHost := extractHost(serverAddr)
		tunnelPort := authResult.TunnelPort
		if tunnelPort == 0 {
			tunnelPort = 7000 // 默认端口
		}
		tunnelAddr = fmt.Sprintf("%s:%d", tunnelHost, tunnelPort)
		log.Printf("使用推导的隧道地址: %s (从 Server 地址 %s)", tunnelAddr, serverAddr)
	}

	token := authResult.TunnelToken
	if token == "" {
		log.Printf("Warning: 隧道 token 未提供，使用默认值")
		token = "awecloud-frp-secret-token-2024"
	}

	log.Printf("隧道配置: addr=%s, token=%s...", tunnelAddr, token[:10])

	// 从 tunnelAddr 中提取主机和端口（用于 FRP 客户端）
	tunnelHost := extractHost(tunnelAddr)

	a.desktopFRP = frp.NewDesktopFRP(tunnelHost, token, a.commandChan, a.statusChan)
	if err := a.desktopFRP.Start(); err != nil {
		a.desktopClient.Stop()
		a.desktopClient = nil
		return fmt.Errorf("failed to start FRP client: %w", err)
	}

	return nil
}

// extractHost 从地址中提取主机名
func extractHost(serverAddr string) string {
	for i := len(serverAddr) - 1; i >= 0; i-- {
		if serverAddr[i] == ':' {
			return serverAddr[:i]
		}
	}
	return serverAddr
}

// Logout 用户登出
func (a *App) Logout() {
	if a.desktopClient != nil {
		a.desktopClient.Stop()
		a.desktopClient = nil
	}
	if a.desktopFRP != nil {
		a.desktopFRP.Stop()
		a.desktopFRP = nil
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
	Version   string `json:"version"`
	GitCommit string `json:"gitCommit"`
	BuildDate string `json:"buildDate"`
}

// GetVersion 获取版本信息
func (a *App) GetVersion() *VersionInfo {
	return &VersionInfo{
		Version:   version,
		GitCommit: gitCommit,
		BuildDate: buildDate,
	}
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

	// TODO: 调用Server API让设备下线
	log.Printf("Offline device: %s", deviceToken)
	return nil
}

// DeleteDevice 删除设备记录
func (a *App) DeleteDevice(deviceToken string) error {
	if a.desktopClient == nil {
		return fmt.Errorf("not logged in")
	}

	// TODO: 调用Server API删除设备
	log.Printf("Delete device: %s", deviceToken)
	return nil
}

// 日志缓冲区
var (
	logBuffer   []string
	logMutex    sync.Mutex
	maxLogLines = 1000
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
