package frp

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fatedier/frp/client"
	v1 "github.com/fatedier/frp/pkg/config/v1"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/models"
)

// DesktopFRP 是 Desktop-FRP 线程，负责 FRP 客户端管理
type DesktopFRP struct {
	serverAddr string // FRP Server 地址，例如 "localhost:7000"
	frpToken   string // FRP 认证 Token

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
func NewDesktopFRP(serverAddr string, frpToken string, commandChan chan *models.VisitorCommand, statusChan chan *models.VisitorStatus) *DesktopFRP {
	ctx, cancel := context.WithCancel(context.Background())
	return &DesktopFRP{
		serverAddr:  serverAddr,
		frpToken:    frpToken,
		services:    make(map[string]*client.Service),
		commandChan: commandChan,
		statusChan:  statusChan,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start 启动 Desktop-FRP 线程
func (f *DesktopFRP) Start() error {
	log.Printf("[Desktop-FRP] Started, server: %s, token: %s", f.serverAddr, f.frpToken[:10]+"...")

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

	// 解析隧道服务器地址
	// 如果命令中指定了服务器地址，使用指定的地址
	// 否则使用默认的 serverAddr:7000
	serverAddr := f.serverAddr
	serverPort := 7000
	websocketPath := "/~!frp" // 默认路径
	protocol := "websocket"

	if cmd.ServerURL != "" {
		// 解析 URL（如 wss://signaling.example.com/ws）
		parsedURL, err := parseServerURL(cmd.ServerURL)
		if err != nil {
			return fmt.Errorf("failed to parse server URL: %w", err)
		}
		serverAddr = parsedURL.Host
		serverPort = parsedURL.Port
		websocketPath = parsedURL.Path
		protocol = parsedURL.Protocol
		log.Printf("[Desktop-FRP] Using server from command: %s (path: %s)", cmd.ServerURL, websocketPath)
	}

	// 创建基础 FRP 配置
	cfg := &v1.ClientCommonConfig{
		ServerAddr: serverAddr,
		ServerPort: serverPort,
		Auth: v1.AuthClientConfig{
			Method: "token",
			Token:  f.frpToken,
		},
		Transport: v1.ClientTransportConfig{
			Protocol: protocol,
		},
	}

	// 创建 FRP Service（每个 Visitor 一个独立的 Service）
	var visitorConfigurer v1.VisitorConfigurer = visitorCfg

	// 使用自定义 Connector 支持自定义 WebSocket 路径
	svr, err := client.NewService(client.ServiceOptions{
		Common:         cfg,
		ProxyCfgs:      nil,
		VisitorCfgs:    []v1.VisitorConfigurer{visitorConfigurer},
		ConfigFilePath: "",
		ConnectorCreator: func(ctx context.Context, cfg *v1.ClientCommonConfig) client.Connector {
			// 使用自定义 connector，支持自定义 WebSocket path
			connector, err := NewCustomConnector(ctx, cfg, websocketPath)
			if err != nil {
				log.Printf("[Desktop-FRP] 创建自定义 Connector 失败: %v，使用默认 Connector", err)
				return client.NewConnector(ctx, cfg)
			}
			return connector
		},
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
	log.Printf("  - Server: %s:%d", serverAddr, serverPort)
	log.Printf("  - Local Port: %d", cmd.LocalPort)
	log.Printf("  - Server Name: %s", cmd.InstanceName)
	log.Printf("  - Secret Key: %s", cmd.SecretKey[:10]+"...")
	log.Printf("  - Token: %s", f.frpToken[:10]+"...")

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

// parseServerURL 解析 FRP Server URL
func parseServerURL(serverURL string) (*struct {
	Host     string
	Port     int
	Path     string
	Protocol string
}, error) {
	// 如果没有协议前缀，添加默认的 ws://
	if !strings.Contains(serverURL, "://") {
		serverURL = "ws://" + serverURL
	}

	parsedURL, err := url.Parse(serverURL)
	if err != nil {
		return nil, err
	}

	result := &struct {
		Host     string
		Port     int
		Path     string
		Protocol string
	}{
		Host: parsedURL.Hostname(),
		Path: parsedURL.Path,
	}

	// 如果路径为空，使用FRP原生路径
	if result.Path == "" {
		result.Path = "/~!frp"
	}

	// 提取端口
	if parsedURL.Port() != "" {
		port, err := strconv.Atoi(parsedURL.Port())
		if err != nil {
			return nil, fmt.Errorf("解析端口失败: %w", err)
		}
		result.Port = port
	} else {
		// 根据协议设置默认端口
		if parsedURL.Scheme == "wss" || parsedURL.Scheme == "https" {
			result.Port = 443
		} else {
			result.Port = 80
		}
	}

	// 确定协议
	if parsedURL.Scheme == "wss" || parsedURL.Scheme == "https" {
		result.Protocol = "wss"
	} else if parsedURL.Scheme == "ws" || parsedURL.Scheme == "http" {
		result.Protocol = "websocket"
	} else {
		result.Protocol = "websocket" // 默认
	}

	return result, nil
}
