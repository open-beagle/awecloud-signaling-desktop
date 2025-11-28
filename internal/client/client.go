package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/models"
	pb "github.com/open-beagle/awecloud-signaling-desktop/pkg/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// DesktopClient 是 Desktop-Web 线程，负责 gRPC 通信
type DesktopClient struct {
	serverAddr string

	// gRPC 连接
	grpcConn   *grpc.ClientConn
	grpcClient pb.ClientServiceClient

	// 认证信息
	sessionToken string
	clientID     string
	frpToken     string // FRP 认证 Token
	frpServer    string // FRP 服务器地址
	frpPort      int32  // FRP 服务器端口

	// 服务列表
	services      map[int64]*models.ServiceInfo
	servicesMutex sync.RWMutex

	// 命令通道（发送给 Desktop-FRP）
	commandChan chan *models.VisitorCommand

	// 状态通道（接收自 Desktop-FRP）
	statusChan chan *models.VisitorStatus

	// 上下文
	ctx    context.Context
	cancel context.CancelFunc
}

// NewDesktopClient 创建 Desktop-Web 线程
func NewDesktopClient(serverAddr string, commandChan chan *models.VisitorCommand, statusChan chan *models.VisitorStatus) *DesktopClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &DesktopClient{
		serverAddr:  serverAddr,
		services:    make(map[int64]*models.ServiceInfo),
		commandChan: commandChan,
		statusChan:  statusChan,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start 启动 Desktop-Web 线程
func (c *DesktopClient) Start() error {
	// 解析 Server 地址，支持 URL 格式
	// 格式: https://signaling.example.com 或 http://localhost:8080
	serverAddr, useTLS, err := parseServerAddress(c.serverAddr)
	if err != nil {
		return fmt.Errorf("failed to parse server address: %w", err)
	}

	// 根据协议选择传输凭证
	var opts []grpc.DialOption
	if useTLS {
		// HTTPS：使用 TLS，跳过证书验证（支持自签名证书）
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
		}
		creds := credentials.NewTLS(tlsConfig)
		opts = append(opts, grpc.WithTransportCredentials(creds))
		log.Printf("[Desktop-Web] Using TLS connection (skip verify)")
	} else {
		// HTTP：不使用 TLS
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		log.Printf("[Desktop-Web] Using plaintext connection")
	}

	// 连接 gRPC Server
	conn, err := grpc.NewClient(serverAddr, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}

	c.grpcConn = conn
	c.grpcClient = pb.NewClientServiceClient(conn)

	log.Printf("[Desktop-Web] Connected to server: %s", serverAddr)

	// 启动状态监听 goroutine
	go c.statusListener()

	return nil
}

// Stop 停止 Desktop-Web 线程
func (c *DesktopClient) Stop() {
	c.cancel()
	if c.grpcConn != nil {
		c.grpcConn.Close()
	}
}

// Authenticate 进行 Client 认证
func (c *DesktopClient) Authenticate(clientID, clientSecret string) error {
	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()

	resp, err := c.grpcClient.Authenticate(ctx, &pb.AuthRequest{
		ClientId:     clientID,
		ClientSecret: clientSecret,
	})
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	c.sessionToken = resp.SessionToken
	c.clientID = clientID
	c.frpToken = resp.Token
	c.frpServer = resp.Server
	c.frpPort = resp.Port

	log.Printf("[Desktop-Web] Authenticated as: %s", clientID)
	return nil
}

// GetServices 获取可访问的服务列表
func (c *DesktopClient) GetServices() ([]*models.ServiceInfo, error) {
	if c.sessionToken == "" {
		return nil, fmt.Errorf("not authenticated")
	}

	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()

	resp, err := c.grpcClient.GetServices(ctx, &pb.GetServicesRequest{
		SessionToken: c.sessionToken,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get services: %w", err)
	}

	// 更新服务列表
	c.servicesMutex.Lock()
	defer c.servicesMutex.Unlock()

	c.services = make(map[int64]*models.ServiceInfo)
	var services []*models.ServiceInfo

	for _, svc := range resp.Services {
		service := &models.ServiceInfo{
			InstanceID:   svc.InstanceId,
			InstanceName: svc.InstanceName,
			AgentName:    svc.AgentName,
			Description:  svc.Description,
			ServicePort:  int(svc.LocalPort),
			AccessType:   svc.AccessType,
			Status:       svc.Status,
			// SecretKey 需要通过 ConnectService 获取
		}
		c.services[service.InstanceID] = service
		services = append(services, service)
	}

	log.Printf("[Desktop-Web] Got %d services", len(services))
	return services, nil
}

// ConnectService 连接到服务
func (c *DesktopClient) ConnectService(instanceID int64, localPort int) error {
	if c.sessionToken == "" {
		return fmt.Errorf("not authenticated")
	}

	// 检查服务是否存在
	c.servicesMutex.RLock()
	_, ok := c.services[instanceID]
	c.servicesMutex.RUnlock()

	if !ok {
		return fmt.Errorf("service not found: %d", instanceID)
	}

	// 调用 ConnectService gRPC 获取 SecretKey
	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()

	resp, err := c.grpcClient.ConnectService(ctx, &pb.ConnectRequest{
		SessionToken: c.sessionToken,
		InstanceId:   instanceID,
	})
	if err != nil {
		return fmt.Errorf("failed to connect service: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("connect failed: %s", resp.Message)
	}

	// 如果没有指定本地端口，使用建议的端口
	if localPort == 0 {
		localPort = int(resp.SuggestedLocalPort)
	}

	// 创建命令
	cmd := &models.VisitorCommand{
		Action:       "connect",
		InstanceID:   instanceID,
		InstanceName: resp.InstanceName,
		SecretKey:    resp.SecretKey,
		LocalPort:    localPort,
		ServerURL:    resp.ServerUrl, // 使用 Server 返回的隧道地址
		Response:     make(chan error, 1),
	}

	// 发送命令到 Desktop-FRP
	select {
	case c.commandChan <- cmd:
		// 等待响应
		select {
		case err := <-cmd.Response:
			return err
		case <-time.After(30 * time.Second):
			return fmt.Errorf("connect timeout")
		}
	case <-time.After(5 * time.Second):
		return fmt.Errorf("command channel full")
	}
}

// DisconnectService 断开服务连接
func (c *DesktopClient) DisconnectService(instanceID int64) error {
	// 获取服务信息
	c.servicesMutex.RLock()
	service, ok := c.services[instanceID]
	c.servicesMutex.RUnlock()

	if !ok {
		return fmt.Errorf("service not found: %d", instanceID)
	}

	// 创建命令
	cmd := &models.VisitorCommand{
		Action:       "disconnect",
		InstanceID:   instanceID,
		InstanceName: service.InstanceName,
		Response:     make(chan error, 1),
	}

	// 发送命令到 Desktop-FRP
	select {
	case c.commandChan <- cmd:
		// 等待响应
		select {
		case err := <-cmd.Response:
			return err
		case <-time.After(10 * time.Second):
			return fmt.Errorf("disconnect timeout")
		}
	case <-time.After(5 * time.Second):
		return fmt.Errorf("command channel full")
	}
}

// statusListener 监听来自 Desktop-FRP 的状态更新
func (c *DesktopClient) statusListener() {
	for {
		select {
		case status := <-c.statusChan:
			log.Printf("[Desktop-Web] Status update: %s - %s", status.InstanceName, status.Status)
			// 这里可以通知前端更新 UI
		case <-c.ctx.Done():
			return
		}
	}
}

// GetSessionToken 返回当前的 session token
func (c *DesktopClient) GetSessionToken() string {
	return c.sessionToken
}

// GetFRPToken 返回 FRP 认证 Token
func (c *DesktopClient) GetFRPToken() string {
	return c.frpToken
}

// GetFRPServer 返回 FRP 服务器地址
func (c *DesktopClient) GetFRPServer() string {
	return c.frpServer
}

// GetFRPPort 返回 FRP 服务器端口
func (c *DesktopClient) GetFRPPort() int32 {
	return c.frpPort
}

// IsAuthenticated 检查是否已认证
func (c *DesktopClient) IsAuthenticated() bool {
	return c.sessionToken != ""
}

// parseServerAddress 解析 Server 地址
// 返回: (grpcAddr, useTLS, error)
func parseServerAddress(serverAddr string) (string, bool, error) {
	// 如果没有协议前缀，添加默认的 http://
	if !strings.Contains(serverAddr, "://") {
		serverAddr = "http://" + serverAddr
	}

	parsedURL, err := url.Parse(serverAddr)
	if err != nil {
		return "", false, err
	}

	// 构建 gRPC 连接地址（host:port）
	var grpcAddr string
	if parsedURL.Port() != "" {
		// URL中指定了端口
		grpcAddr = parsedURL.Host
	} else {
		// URL中没有端口，使用协议默认端口
		port := 80
		if parsedURL.Scheme == "https" {
			port = 443
		}
		grpcAddr = fmt.Sprintf("%s:%d", parsedURL.Hostname(), port)
	}

	useTLS := parsedURL.Scheme == "https"
	return grpcAddr, useTLS, nil
}
