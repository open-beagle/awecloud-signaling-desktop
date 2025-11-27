package frp

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"awecloud-desktop/internal/models"

	"github.com/fatedier/frp/client"
	v1 "github.com/fatedier/frp/pkg/config/v1"
)

// DesktopFRP 是 Desktop-FRP 线程，负责 FRP 客户端管理
type DesktopFRP struct {
	serverAddr string // Server 地址，例如 "localhost:7000"
	token      string // 认证 Token

	// 每个 Visitor 对应一个独立的 FRP Service
	services map[string]*client.Service
	mutex    sync.RWMutex

	// 命令通道（接收自 Desktop-Web）
	commandChan chan *models.VisitorCommand

	// 状态通道（发送给 Desktop-Web）
	statusChan chan *models.VisitorStatus

	// 上下文
	ctx    context.Context
	cancel context.CancelFunc
}

// NewDesktopFRP 创建 Desktop-FRP 线程
func NewDesktopFRP(serverAddr string, token string, commandChan chan *models.VisitorCommand, statusChan chan *models.VisitorStatus) *DesktopFRP {
	ctx, cancel := context.WithCancel(context.Background())
	return &DesktopFRP{
		serverAddr:  serverAddr,
		token:       token,
		services:    make(map[string]*client.Service),
		commandChan: commandChan,
		statusChan:  statusChan,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start 启动 Desktop-FRP 线程
func (f *DesktopFRP) Start() error {
	log.Printf("[Desktop-FRP] Started, server: %s:7000, token: %s", f.serverAddr, f.token[:10]+"...")

	// 启动命令处理 goroutine
	go f.commandHandler()

	return nil
}

// Stop 停止 Desktop-FRP 线程
func (f *DesktopFRP) Stop() {
	f.cancel()

	// 关闭所有 FRP Service
	f.mutex.Lock()
	defer f.mutex.Unlock()

	for name, service := range f.services {
		log.Printf("[Desktop-FRP] Closing service: %s", name)
		service.Close()
	}
	f.services = make(map[string]*client.Service)
}

// commandHandler 处理来自 Desktop-Web 的命令
func (f *DesktopFRP) commandHandler() {
	for {
		select {
		case cmd := <-f.commandChan:
			var err error
			switch cmd.Action {
			case "connect":
				err = f.addVisitor(cmd)
			case "disconnect":
				err = f.removeVisitor(cmd)
			default:
				err = fmt.Errorf("unknown command: %s", cmd.Action)
			}

			// 发送响应
			if cmd.Response != nil {
				cmd.Response <- err
			}

		case <-f.ctx.Done():
			return
		}
	}
}

// addVisitor 添加 STCP Visitor
func (f *DesktopFRP) addVisitor(cmd *models.VisitorCommand) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	visitorName := cmd.InstanceName + "-visitor"

	// 检查是否已存在
	if _, exists := f.services[visitorName]; exists {
		return fmt.Errorf("visitor already exists: %s", visitorName)
	}

	// 创建 STCP Visitor 配置
	visitorCfg := &v1.STCPVisitorConfig{
		VisitorBaseConfig: v1.VisitorBaseConfig{
			Name:       visitorName,
			Type:       "stcp",
			ServerName: cmd.InstanceName, // 对应 Agent 端的 STCP Proxy 名称
			SecretKey:  cmd.SecretKey,
			BindAddr:   "127.0.0.1",
			BindPort:   cmd.LocalPort,
		},
	}

	// 创建基础 FRP 配置
	cfg := &v1.ClientCommonConfig{
		ServerAddr: f.serverAddr,
		ServerPort: 7000,
		Auth: v1.AuthClientConfig{
			Method: "token",
			Token:  f.token,
		},
		Transport: v1.ClientTransportConfig{
			Protocol: "websocket",
		},
	}

	// 创建 FRP Service（每个 Visitor 一个独立的 Service）
	var visitorConfigurer v1.VisitorConfigurer = visitorCfg
	svr, err := client.NewService(client.ServiceOptions{
		Common:         cfg,
		ProxyCfgs:      nil,
		VisitorCfgs:    []v1.VisitorConfigurer{visitorConfigurer},
		ConfigFilePath: "",
	})
	if err != nil {
		return fmt.Errorf("failed to create FRP service: %w", err)
	}

	// 启动 FRP Service
	go func() {
		if err := svr.Run(f.ctx); err != nil {
			log.Printf("[Desktop-FRP] FRP service error for %s: %v", visitorName, err)
		}
	}()

	// 保存 Service
	f.services[visitorName] = svr

	log.Printf("[Desktop-FRP] Added visitor: %s", visitorName)
	log.Printf("  - Server: %s:7000", f.serverAddr)
	log.Printf("  - Local Port: %d", cmd.LocalPort)
	log.Printf("  - Server Name: %s", cmd.InstanceName)
	log.Printf("  - Secret Key: %s", cmd.SecretKey[:10]+"...")
	log.Printf("  - Token: %s", f.token[:10]+"...")

	// 发送状态更新
	f.sendStatus(&models.VisitorStatus{
		InstanceID:   cmd.InstanceID,
		InstanceName: cmd.InstanceName,
		Status:       "connected",
		LocalPort:    cmd.LocalPort,
	})

	return nil
}

// removeVisitor 移除 STCP Visitor
func (f *DesktopFRP) removeVisitor(cmd *models.VisitorCommand) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	visitorName := cmd.InstanceName + "-visitor"

	// 检查是否存在
	service, exists := f.services[visitorName]
	if !exists {
		return fmt.Errorf("visitor not found: %s", visitorName)
	}

	// 关闭 FRP Service
	service.Close()

	// 删除 Service
	delete(f.services, visitorName)

	log.Printf("[Desktop-FRP] Removed visitor: %s", visitorName)

	// 发送状态更新
	f.sendStatus(&models.VisitorStatus{
		InstanceID:   cmd.InstanceID,
		InstanceName: cmd.InstanceName,
		Status:       "disconnected",
	})

	return nil
}

// sendStatus 发送状态更新到 Desktop-Web
func (f *DesktopFRP) sendStatus(status *models.VisitorStatus) {
	select {
	case f.statusChan <- status:
	case <-time.After(1 * time.Second):
		log.Printf("[Desktop-FRP] Failed to send status: channel full")
	}
}

// GetVisitors 返回当前的 Visitor 列表
func (f *DesktopFRP) GetVisitors() []string {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	visitors := make([]string, 0, len(f.services))
	for name := range f.services {
		visitors = append(visitors, name)
	}
	return visitors
}
