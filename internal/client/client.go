package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/device"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/models"
	pb "github.com/open-beagle/awecloud-signaling-desktop/pkg/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// DesktopClient 是 Desktop-Web 线程，负责 gRPC 通信
type DesktopClient struct {
	serverAddr string // gRPC地址（去掉协议前缀）
	serverURL  string // 完整的服务器URL（包含协议）

	// gRPC 连接
	grpcConn   *grpc.ClientConn
	grpcClient pb.ClientServiceClient

	// 审计日志客户端
	auditClient *AuditClient

	// 设备管理客户端
	deviceClient *DeviceClient

	// 隧道配置客户端
	tunnelConfigClient *TunnelConfigClient

	// 认证信息
	sessionToken string
	clientID     string

	// 服务列表
	services      map[int64]*models.ServiceInfo
	servicesMutex sync.RWMutex

	// 命令通道（发送给 Desktop-Tunnel）
	commandChan chan *models.VisitorCommand

	// 状态通道（接收自 Desktop-Tunnel）
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
		serverURL:   serverAddr, // 保存原始URL
		services:    make(map[int64]*models.ServiceInfo),
		commandChan: commandChan,
		statusChan:  statusChan,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start 启动 Desktop-Web 线程
func (c *DesktopClient) Start() error {
	// 规范化服务器地址（移除末尾的斜杠）
	c.serverAddr = strings.TrimSuffix(c.serverAddr, "/")

	// 保存原始URL（包含协议）
	c.serverURL = c.serverAddr

	// 根据地址判断是否使用 TLS
	var opts []grpc.DialOption

	if strings.HasPrefix(c.serverAddr, "https://") {
		// HTTPS：使用 TLS，跳过证书验证
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
		log.Printf("[Desktop-Web] Using TLS connection (skip verify)")

		// 移除 https:// 前缀
		c.serverAddr = strings.TrimPrefix(c.serverAddr, "https://")
	} else if strings.HasPrefix(c.serverAddr, "http://") {
		// HTTP：不使用 TLS
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		log.Printf("[Desktop-Web] Using plaintext connection")

		// 移除 http:// 前缀
		c.serverAddr = strings.TrimPrefix(c.serverAddr, "http://")
	} else {
		// 没有协议前缀，默认使用 plaintext
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		log.Printf("[Desktop-Web] Using plaintext connection (no protocol specified)")
		// 为没有协议的地址添加 http:// 前缀
		c.serverURL = "http://" + c.serverAddr
	}

	// 连接 gRPC Server
	conn, err := grpc.NewClient(c.serverAddr, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}

	c.grpcConn = conn
	c.grpcClient = pb.NewClientServiceClient(conn)

	log.Printf("[Desktop-Web] Connected to server: %s", c.serverAddr)

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
		log.Printf("[Desktop-Web] GetServices error: %v", err)
		return nil, fmt.Errorf("failed to get services: %w", err)
	}

	log.Printf("[Desktop-Web] GetServices response: success=%v, services_count=%d", resp.Success, len(resp.Services))

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
			ServiceIP:    svc.LocalIp,
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

	// 处理ServerURL
	serverURL := resp.ServerUrl
	if serverURL == "" {
		// 如果Server没有返回URL，使用连接的Server地址 + 默认端口7000
		// 从serverURL中提取主机名（移除协议和端口）
		host := c.serverURL
		host = strings.TrimPrefix(host, "https://")
		host = strings.TrimPrefix(host, "http://")
		host = strings.TrimPrefix(host, "wss://")
		host = strings.TrimPrefix(host, "ws://")
		if idx := strings.Index(host, ":"); idx != -1 {
			host = host[:idx]
		}
		if idx := strings.Index(host, "/"); idx != -1 {
			host = host[:idx]
		}
		serverURL = fmt.Sprintf("ws://%s:7000", host)
		log.Printf("[Desktop-Web] Server returned empty URL, using: %s", serverURL)
	}

	// 创建命令
	cmd := &models.VisitorCommand{
		Action:       "connect",
		InstanceID:   instanceID,
		InstanceName: resp.InstanceName,
		SecretKey:    resp.SecretKey,
		LocalPort:    localPort,
		ServerURL:    serverURL, // 使用处理后的隧道地址
		Response:     make(chan error, 1),
	}

	// 发送命令到 Desktop-Tunnel
	select {
	case c.commandChan <- cmd:
		// 等待响应
		select {
		case err := <-cmd.Response:
			// 记录审计日志
			c.recordAuditLog(instanceID, "connect", localPort, err == nil, err)
			return err
		case <-time.After(30 * time.Second):
			err := fmt.Errorf("connect timeout")
			c.recordAuditLog(instanceID, "connect", localPort, false, err)
			return err
		}
	case <-time.After(5 * time.Second):
		err := fmt.Errorf("command channel full")
		c.recordAuditLog(instanceID, "connect", localPort, false, err)
		return err
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

	// 发送命令到 Desktop-Tunnel
	select {
	case c.commandChan <- cmd:
		// 等待响应
		select {
		case err := <-cmd.Response:
			// 记录审计日志
			c.recordAuditLog(instanceID, "disconnect", 0, err == nil, err)
			return err
		case <-time.After(10 * time.Second):
			err := fmt.Errorf("disconnect timeout")
			c.recordAuditLog(instanceID, "disconnect", 0, false, err)
			return err
		}
	case <-time.After(5 * time.Second):
		return fmt.Errorf("command channel full")
	}
}

// statusListener 监听来自 Desktop-Tunnel 的状态更新
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

// IsAuthenticated 检查是否已认证
func (c *DesktopClient) IsAuthenticated() bool {
	return c.sessionToken != ""
}

// recordAuditLog 记录审计日志（异步，不阻塞主流程）
func (c *DesktopClient) recordAuditLog(instanceID int64, action string, localPort int, success bool, err error) {
	// 异步记录，避免阻塞
	go func() {
		if c.auditClient == nil {
			log.Printf("[Audit] Audit client not initialized, skipping log")
			return
		}

		// 获取设备指纹和信息
		fingerprint, fpErr := device.GetFingerprint()
		if fpErr != nil {
			log.Printf("[Audit] Failed to get device fingerprint: %v", fpErr)
			return
		}

		// 构建错误消息
		errorMessage := ""
		if err != nil {
			errorMessage = err.Error()
		}

		// 构建设备信息
		deviceInfo := DeviceInfo{
			OS:        fingerprint.OS,
			OSVersion: device.GetOSInfo(),
			Arch:      fingerprint.Arch,
			CPUModel:  "",
			MachineID: fingerprint.MachineID,
			Hostname:  fingerprint.Hostname,
		}

		// 记录审计日志
		req := &RecordConnectionRequest{
			STCPInstanceID:    instanceID,
			Action:            action,
			LocalPort:         localPort,
			DeviceFingerprint: fingerprint.Hash,
			DeviceInfo:        deviceInfo,
			Success:           success,
			ErrorMessage:      errorMessage,
			ServerAddress:     c.serverAddr, // Desktop连接的Server地址
		}

		if err := c.auditClient.RecordConnection(req); err != nil {
			log.Printf("[Audit] Failed to record audit log: %v", err)
		} else {
			log.Printf("[Audit] Recorded %s action for instance %d (success=%v)", action, instanceID, success)
		}
	}()
}

// DeviceInfoResult 设备信息结果（用于返回给App层）
type DeviceInfoResult struct {
	DeviceToken string
	DeviceName  string
	OS          string
	Arch        string
	Hostname    string
	Status      string
	LastUsedAt  string
	CreatedAt   string
	IsCurrent   bool
}

// GetDevices 获取已登录的设备列表
func (c *DesktopClient) GetDevices() ([]*DeviceInfoResult, error) {
	if c.sessionToken == "" {
		return nil, fmt.Errorf("not authenticated")
	}

	if c.deviceClient == nil {
		return nil, fmt.Errorf("device client not initialized")
	}

	// 调用Server API获取设备列表
	devices, err := c.deviceClient.ListDevices()
	if err != nil {
		return nil, fmt.Errorf("failed to list devices: %w", err)
	}

	// 转换为DeviceInfoResult
	results := make([]*DeviceInfoResult, 0, len(devices))
	for _, d := range devices {
		// 构建设备名称
		deviceName := fmt.Sprintf("%s %s", d.DeviceInfo.OS, d.DeviceInfo.Hostname)

		// 确定状态
		status := "offline"
		if !d.Revoked {
			// 检查是否过期
			expiresAt, err := time.Parse(time.RFC3339, d.ExpiresAt)
			if err == nil && time.Now().Before(expiresAt) {
				status = "online"
			}
		}

		results = append(results, &DeviceInfoResult{
			DeviceToken: d.DeviceToken,
			DeviceName:  deviceName,
			OS:          d.DeviceInfo.OS,
			Arch:        d.DeviceInfo.Arch,
			Hostname:    d.DeviceInfo.Hostname,
			Status:      status,
			LastUsedAt:  d.LastUsedAt,
			CreatedAt:   d.CreatedAt,
			IsCurrent:   d.IsCurrent,
		})
	}

	return results, nil
}

// OfflineDevice 让设备下线
func (c *DesktopClient) OfflineDevice(deviceToken string) error {
	if c.sessionToken == "" {
		return fmt.Errorf("not authenticated")
	}

	if c.deviceClient == nil {
		return fmt.Errorf("device client not initialized")
	}

	return c.deviceClient.OfflineDevice(deviceToken)
}

// DeleteDevice 删除设备记录
func (c *DesktopClient) DeleteDevice(deviceToken string) error {
	if c.sessionToken == "" {
		return fmt.Errorf("not authenticated")
	}

	if c.deviceClient == nil {
		return fmt.Errorf("device client not initialized")
	}

	return c.deviceClient.DeleteDevice(deviceToken)
}

// GetTunnelConfig 获取隧道配置
func (c *DesktopClient) GetTunnelConfig() (*TunnelConfigResponse, error) {
	if c.sessionToken == "" {
		return nil, fmt.Errorf("not authenticated")
	}

	if c.tunnelConfigClient == nil {
		return nil, fmt.Errorf("tunnel config client not initialized")
	}

	return c.tunnelConfigClient.GetTunnelConfig()
}
