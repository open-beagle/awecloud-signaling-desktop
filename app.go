package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
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
		// 使用编译时注入的默认地址，如果没有则使用 localhost
		serverAddr := defaultServerAddress
		if serverAddr == "" {
			serverAddr = "http://localhost:9090"
		}
		cfg = &config.Config{
			ServerAddress:   serverAddr,
			RememberMe:      true,
			PortPreferences: make(map[int64]int),
		}
	}
	a.config = cfg

	log.Printf("Desktop app started")
	log.Printf("Server address: %s", a.config.ServerAddress)
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
	// 更新配置
	a.config.ServerAddress = serverAddr
	a.config.ClientID = clientID
	a.config.RememberMe = rememberMe

	// 如果勾选记住登录，保存凭据和设置过期时间（7天）
	if rememberMe {
		a.config.ClientSecret = clientSecret
		a.config.TokenExpiresAt = time.Now().Add(7 * 24 * time.Hour).Unix()
	} else {
		a.config.ClientSecret = ""
		a.config.TokenExpiresAt = 0
	}

	// 保存配置
	if err := a.config.Save(); err != nil {
		log.Printf("Failed to save config: %v", err)
	}

	// 创建 Desktop-Web 线程
	a.desktopClient = client.NewDesktopClient(serverAddr, a.commandChan, a.statusChan)
	if err := a.desktopClient.Start(); err != nil {
		return fmt.Errorf("failed to start desktop client: %w", err)
	}

	// 认证
	if err := a.desktopClient.Authenticate(clientID, clientSecret); err != nil {
		a.desktopClient.Stop()
		a.desktopClient = nil
		return fmt.Errorf("authentication failed: %w", err)
	}

	// 创建 Desktop-FRP 线程
	// 从 serverAddr 提取主机名（支持 URL 格式）
	host, err := extractHostname(serverAddr)
	if err != nil {
		a.desktopClient.Stop()
		a.desktopClient = nil
		return fmt.Errorf("failed to extract hostname: %w", err)
	}

	// FRP Token（从 Server 获取）
	frpToken := a.desktopClient.GetFRPToken()
	if frpToken == "" {
		log.Println("[Desktop-FRP] Warning: FRP token is empty, server may not require authentication")
	}
	a.desktopFRP = frp.NewDesktopFRP(host, frpToken, a.commandChan, a.statusChan)
	if err := a.desktopFRP.Start(); err != nil {
		a.desktopClient.Stop()
		a.desktopClient = nil
		return fmt.Errorf("failed to start FRP client: %w", err)
	}

	return nil
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
	if a.desktopClient == nil {
		return nil, fmt.Errorf("not logged in")
	}

	services, err := a.desktopClient.GetServices()
	if err != nil {
		return nil, err
	}

	// 为每个服务添加偏好端口（不修改 ServicePort）
	for _, service := range services {
		if preferredPort, exists := a.config.PortPreferences[service.InstanceID]; exists {
			service.PreferredPort = preferredPort
		} else {
			// 如果没有偏好端口，默认使用服务端口
			service.PreferredPort = service.ServicePort
		}
	}

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
	// 检查是否记住登录
	if !a.config.RememberMe {
		return nil
	}

	// 检查凭据是否过期
	if a.config.TokenExpiresAt > 0 && time.Now().Unix() > a.config.TokenExpiresAt {
		// 凭据已过期，清除
		a.config.ClientSecret = ""
		a.config.TokenExpiresAt = 0
		a.config.RememberMe = false
		a.config.Save()
		return nil
	}

	// 检查是否有完整的凭据
	if a.config.ServerAddress == "" || a.config.ClientID == "" || a.config.ClientSecret == "" {
		return nil
	}

	return &SavedCredentials{
		ServerAddress: a.config.ServerAddress,
		ClientID:      a.config.ClientID,
		ClientSecret:  a.config.ClientSecret,
		RememberMe:    a.config.RememberMe,
	}
}

// SavedCredentials 保存的凭据
type SavedCredentials struct {
	ServerAddress string `json:"server_address"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
	RememberMe    bool   `json:"remember_me"`
}

// GetLogs 获取日志（最近 100 行）
func (a *App) GetLogs() []string {
	return GetRecentLogs(100)
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

// extractHostname 从 Server 地址提取主机名
func extractHostname(serverAddr string) (string, error) {
	// 如果没有协议前缀，添加默认的 http://
	if !strings.Contains(serverAddr, "://") {
		serverAddr = "http://" + serverAddr
	}

	parsedURL, err := url.Parse(serverAddr)
	if err != nil {
		return "", err
	}

	return parsedURL.Hostname(), nil
}
