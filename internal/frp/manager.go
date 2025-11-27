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
	serverAddr string // FRP Server 地址，例如 "localhost:7000"

	// FRP 客户端
	service *client.Service

	// Visitor 配置
	visitors map[string]*v1.VisitorConfigurer
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
func NewDesktopFRP(serverAddr string, commandChan chan *models.VisitorCommand, statusChan chan *models.VisitorStatus) *DesktopFRP {
	ctx, cancel := context.WithCancel(context.Background())
	return &DesktopFRP{
		serverAddr:  serverAddr,
		visitors:    make(map[string]*v1.VisitorConfigurer),
		commandChan: commandChan,
		statusChan:  statusChan,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start 启动 Desktop-FRP 线程
func (f *DesktopFRP) Start() error {
	// 创建基础 FRP 配置
	cfg := &v1.ClientCommonConfig{
		ServerAddr: f.serverAddr,
		ServerPort: 7000,
		Transport: v1.ClientTransportConfig{
			Protocol: "websocket",
		},
	}

	// 创建 FRP Service
	svr, err := client.NewService(client.ServiceOptions{
		Common:         cfg,
		ProxyCfgs:      nil, // Desktop 不需要 Proxy，只需要 Visitor
		VisitorCfgs:    nil, // 初始为空，动态添加
		ConfigFilePath: "",
	})
	if err != nil {
		return fmt.Errorf("failed to create FRP service: %w", err)
	}

	f.service = svr

	// 启动 FRP Service
	go func() {
		if err := f.service.Run(f.ctx); err != nil {
			log.Printf("[Desktop-FRP] FRP service error: %v", err)
		}
	}()

	log.Printf("[Desktop-FRP] Started, connecting to: %s:7000", f.serverAddr)

	// 启动命令处理 goroutine
	go f.commandHandler()

	return nil
}

// Stop 停止 Desktop-FRP 线程
func (f *DesktopFRP) Stop() {
	f.cancel()
	if f.service != nil {
		f.service.Close()
	}
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
	if _, exists := f.visitors[visitorName]; exists {
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

	// 保存配置
	var configurer v1.VisitorConfigurer = visitorCfg
	f.visitors[visitorName] = &configurer

	log.Printf("[Desktop-FRP] Added visitor: %s (local port: %d)", visitorName, cmd.LocalPort)

	// 发送状态更新
	f.sendStatus(&models.VisitorStatus{
		InstanceID:   cmd.InstanceID,
		InstanceName: cmd.InstanceName,
		Status:       "connected",
		LocalPort:    cmd.LocalPort,
	})

	// TODO: 实际添加到 FRP Service（需要 FRP 支持动态添加）
	// 目前 FRP 不支持运行时动态添加 Visitor，需要重启 Service
	// 这是一个已知限制，后续版本可以考虑：
	// 1. 使用 FRP 的 API（如果有）
	// 2. 重启 FRP Service
	// 3. 为每个 Visitor 创建独立的 FRP Client

	return nil
}

// removeVisitor 移除 STCP Visitor
func (f *DesktopFRP) removeVisitor(cmd *models.VisitorCommand) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	visitorName := cmd.InstanceName + "-visitor"

	// 检查是否存在
	if _, exists := f.visitors[visitorName]; !exists {
		return fmt.Errorf("visitor not found: %s", visitorName)
	}

	// 删除配置
	delete(f.visitors, visitorName)

	log.Printf("[Desktop-FRP] Removed visitor: %s", visitorName)

	// 发送状态更新
	f.sendStatus(&models.VisitorStatus{
		InstanceID:   cmd.InstanceID,
		InstanceName: cmd.InstanceName,
		Status:       "disconnected",
	})

	// TODO: 实际从 FRP Service 移除（需要 FRP 支持动态移除）

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

	visitors := make([]string, 0, len(f.visitors))
	for name := range f.visitors {
		visitors = append(visitors, name)
	}
	return visitors
}
