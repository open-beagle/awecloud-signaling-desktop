package main

import (
	"context"
	"fmt"
	"log"

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

	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Failed to load config: %v, using defaults", err)
		cfg = &config.Config{
			ServerAddress: "localhost:8081",
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
func (a *App) Login(serverAddr, clientID, clientSecret string) error {
	// 更新配置
	a.config.ServerAddress = serverAddr
	a.config.ClientID = clientID
	a.config.ClientSecret = clientSecret

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
	// 从 serverAddr 提取主机名（假设格式为 "host:port"）
	host := serverAddr
	if idx := len(serverAddr) - 1; idx > 0 {
		for i := len(serverAddr) - 1; i >= 0; i-- {
			if serverAddr[i] == ':' {
				host = serverAddr[:i]
				break
			}
		}
	}

	a.desktopFRP = frp.NewDesktopFRP(host, a.commandChan, a.statusChan)
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
	return a.desktopClient.GetServices()
}

// ConnectService 连接服务
func (a *App) ConnectService(instanceID int64, localPort int) error {
	if a.desktopClient == nil {
		return fmt.Errorf("not logged in")
	}
	return a.desktopClient.ConnectService(instanceID, localPort)
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
