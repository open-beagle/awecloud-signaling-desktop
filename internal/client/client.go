package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/device"
	pb "github.com/open-beagle/awecloud-signaling-desktop/pkg/proto"
)

// DesktopClient Desktop 客户端
type DesktopClient struct {
	serverAddr string // gRPC地址（去掉协议前缀）
	serverURL  string // 完整的服务器URL（包含协议）

	// gRPC 连接
	grpcConn   *grpc.ClientConn
	grpcClient pb.DesktopServiceClient

	// Desktop 信息
	desktopID uint64
	secret    string
	clientID  string
	mu        sync.RWMutex // 保护 desktopID 和 secret

	// 心跳流
	heartbeatStream pb.DesktopService_HeartbeatClient
	heartbeatMutex  sync.Mutex

	// 隧道状态（用于心跳上报）
	tunnelIP        string
	tunnelConnected bool
	tunnelMutex     sync.RWMutex

	// 隧道状态查询回调（用于重连时获取最新状态）
	getTunnelStatus func() (ip string, connected bool)

	// gRPC 连接状态
	grpcConnected bool
	connMutex     sync.RWMutex

	// 重连回调（用于通知 App 层重新获取 authKey）
	onReconnectNeeded func() error

	// 已授权服务列表
	authorizedServices []*pb.AuthorizedService
	servicesMutex      sync.RWMutex

	// 上下文
	ctx    context.Context
	cancel context.CancelFunc
}

// NewDesktopClient 创建 Desktop 客户端
func NewDesktopClient(serverAddr string) *DesktopClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &DesktopClient{
		serverAddr:         serverAddr,
		serverURL:          serverAddr,
		authorizedServices: make([]*pb.AuthorizedService, 0),
		ctx:                ctx,
		cancel:             cancel,
	}
}

// Start 启动客户端
func (c *DesktopClient) Start() error {
	// 规范化服务器地址（移除末尾的斜杠）
	c.serverAddr = strings.TrimSuffix(c.serverAddr, "/")
	c.serverURL = c.serverAddr

	// 根据地址判断是否使用 TLS
	var opts []grpc.DialOption

	if strings.HasPrefix(c.serverAddr, "https://") {
		// HTTPS：使用 TLS，跳过证书验证
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
			// 必须设置 NextProtos 以支持 HTTP/2（gRPC 要求）
			NextProtos: []string{"h2"},
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
		log.Printf("[DesktopClient] Using TLS connection (skip verify)")
		c.serverAddr = strings.TrimPrefix(c.serverAddr, "https://")
	} else if strings.HasPrefix(c.serverAddr, "http://") {
		// HTTP：不使用 TLS
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		log.Printf("[DesktopClient] Using plaintext connection")
		c.serverAddr = strings.TrimPrefix(c.serverAddr, "http://")
	} else {
		// 没有协议前缀，默认使用 plaintext
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		log.Printf("[DesktopClient] Using plaintext connection (no protocol specified)")
		c.serverURL = "http://" + c.serverAddr
	}

	// 连接 gRPC Server
	conn, err := grpc.NewClient(c.serverAddr, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}

	c.grpcConn = conn
	c.grpcClient = pb.NewDesktopServiceClient(conn)

	log.Printf("[DesktopClient] Connected to server: %s", c.serverAddr)
	return nil
}

// Stop 停止客户端
func (c *DesktopClient) Stop() {
	c.cancel()
	if c.grpcConn != nil {
		c.grpcConn.Close()
	}
}

// IsAuthenticated 检查是否已认证
func (c *DesktopClient) IsAuthenticated() bool {
	c.mu.RLock()
	hasCredentials := c.desktopID > 0 && c.secret != ""
	c.mu.RUnlock()
	return hasCredentials
}

// IsGRPCConnected 检查 gRPC 连接是否存活
func (c *DesktopClient) IsGRPCConnected() bool {
	c.connMutex.RLock()
	defer c.connMutex.RUnlock()
	return c.grpcConnected
}

// setGRPCConnected 设置 gRPC 连接状态
func (c *DesktopClient) setGRPCConnected(connected bool) {
	c.connMutex.Lock()
	defer c.connMutex.Unlock()
	c.grpcConnected = connected
}

// SetReconnectCallback 设置重连回调
func (c *DesktopClient) SetReconnectCallback(callback func() error) {
	c.onReconnectNeeded = callback
}

// SetTunnelStatusCallback 设置隧道状态查询回调
func (c *DesktopClient) SetTunnelStatusCallback(callback func() (ip string, connected bool)) {
	c.getTunnelStatus = callback
}

// GetAuthorizedServices 获取已授权服务列表
func (c *DesktopClient) GetAuthorizedServices() []*pb.AuthorizedService {
	c.servicesMutex.RLock()
	defer c.servicesMutex.RUnlock()
	return c.authorizedServices
}

// startHeartbeat 启动心跳
func (c *DesktopClient) startHeartbeat(tunnelIP string, tunnelConnected bool) error {
	c.heartbeatMutex.Lock()
	defer c.heartbeatMutex.Unlock()

	// 更新隧道状态
	if tunnelIP != "" {
		// 传入了有效值，直接使用
		c.tunnelMutex.Lock()
		c.tunnelIP = tunnelIP
		c.tunnelConnected = tunnelConnected
		c.tunnelMutex.Unlock()
	} else if c.getTunnelStatus != nil {
		// 没有传入有效值，尝试通过回调获取最新状态
		ip, connected := c.getTunnelStatus()
		if ip != "" {
			c.tunnelMutex.Lock()
			c.tunnelIP = ip
			c.tunnelConnected = connected
			c.tunnelMutex.Unlock()
		}
	}

	// 如果已有心跳流，先关闭
	if c.heartbeatStream != nil {
		c.heartbeatStream.CloseSend()
		c.heartbeatStream = nil
	}

	// 创建心跳流
	stream, err := c.grpcClient.Heartbeat(c.ctx)
	if err != nil {
		c.setGRPCConnected(false)
		return fmt.Errorf("failed to create heartbeat stream: %w", err)
	}

	c.heartbeatStream = stream
	c.setGRPCConnected(true)

	// 获取当前隧道状态
	c.tunnelMutex.RLock()
	currentIP := c.tunnelIP
	currentConnected := c.tunnelConnected
	c.tunnelMutex.RUnlock()

	// 发送首次心跳
	req := &pb.DesktopHeartbeatRequest{
		DesktopId:       c.desktopID,
		TunnelIp:        currentIP,
		TunnelConnected: currentConnected,
	}

	if err := stream.Send(req); err != nil {
		c.setGRPCConnected(false)
		return fmt.Errorf("failed to send initial heartbeat: %w", err)
	}

	log.Printf("[DesktopClient] Heartbeat started, tunnelIP=%s", currentIP)

	// 启动接收 goroutine
	go c.receiveHeartbeat()

	// 启动发送 goroutine
	go c.sendHeartbeat()

	return nil
}

// receiveHeartbeat 接收心跳响应
func (c *DesktopClient) receiveHeartbeat() {
	backoff := time.Second * 5
	maxBackoff := time.Minute * 2

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			c.heartbeatMutex.Lock()
			stream := c.heartbeatStream
			c.heartbeatMutex.Unlock()

			if stream == nil {
				time.Sleep(time.Second)
				continue
			}

			resp, err := stream.Recv()
			if err != nil {
				log.Printf("[DesktopClient] Heartbeat receive error: %v", err)
				c.setGRPCConnected(false)

				// 尝试重连
				log.Printf("[DesktopClient] Will retry in %v", backoff)
				time.Sleep(backoff)

				// 指数退避
				backoff = backoff * 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}

				// 尝试重新启动心跳
				if c.IsAuthenticated() {
					if err := c.startHeartbeat("", false); err != nil {
						log.Printf("[DesktopClient] Failed to restart heartbeat: %v", err)
					} else {
						log.Printf("[DesktopClient] Heartbeat restarted successfully")
						backoff = time.Second * 5 // 重置退避时间
					}
				}
				continue
			}

			// 连接成功，重置退避时间
			backoff = time.Second * 5
			c.setGRPCConnected(true)

			// 更新已授权服务列表
			c.servicesMutex.Lock()
			c.authorizedServices = resp.AuthorizedServices
			c.servicesMutex.Unlock()

			log.Printf("[DEBUG] [DesktopClient] Heartbeat received, authorized services: %d", len(resp.AuthorizedServices))
		}
	}
}

// sendHeartbeat 发送心跳
func (c *DesktopClient) sendHeartbeat() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.heartbeatMutex.Lock()
			stream := c.heartbeatStream
			c.heartbeatMutex.Unlock()

			if stream == nil {
				continue
			}

			// 获取当前隧道状态
			c.tunnelMutex.RLock()
			tunnelIP := c.tunnelIP
			tunnelConnected := c.tunnelConnected
			c.tunnelMutex.RUnlock()

			req := &pb.DesktopHeartbeatRequest{
				DesktopId:       c.desktopID,
				TunnelIp:        tunnelIP,
				TunnelConnected: tunnelConnected,
			}

			if err := stream.Send(req); err != nil {
				log.Printf("[DesktopClient] Failed to send heartbeat: %v", err)
			}
		}
	}
}

// UpdateHeartbeat 更新心跳信息（当隧道状态变化时调用）
func (c *DesktopClient) UpdateHeartbeat(tunnelIP string, tunnelConnected bool) {
	// 更新隧道状态
	c.tunnelMutex.Lock()
	c.tunnelIP = tunnelIP
	c.tunnelConnected = tunnelConnected
	c.tunnelMutex.Unlock()

	log.Printf("[DesktopClient] Tunnel status updated: ip=%s, connected=%v", tunnelIP, tunnelConnected)

	c.heartbeatMutex.Lock()
	stream := c.heartbeatStream
	c.heartbeatMutex.Unlock()

	if stream == nil {
		// 如果心跳未启动，启动它
		if err := c.startHeartbeat(tunnelIP, tunnelConnected); err != nil {
			log.Printf("[DesktopClient] Failed to start heartbeat: %v", err)
		}
		return
	}

	// 立即发送一次心跳更新
	req := &pb.DesktopHeartbeatRequest{
		DesktopId:       c.desktopID,
		TunnelIp:        tunnelIP,
		TunnelConnected: tunnelConnected,
	}

	if err := stream.Send(req); err != nil {
		log.Printf("[DesktopClient] Failed to update heartbeat: %v", err)
	}
}

// getSystemInfo 获取系统信息
func getSystemInfo() (*pb.DesktopSystemInfo, error) {
	fingerprint, err := device.GetFingerprint()
	if err != nil {
		return nil, fmt.Errorf("failed to get device fingerprint: %w", err)
	}

	return &pb.DesktopSystemInfo{
		Os:        fingerprint.OS,
		OsVersion: device.GetOSInfo(),
		Arch:      fingerprint.Arch,
		Hostname:  fingerprint.Hostname,
		Cpu:       "",
		CpuCores:  0,
		MemoryGb:  0,
	}, nil
}

// HostInfo 主机信息
type HostInfo struct {
	HostID   string   `json:"host_id"`
	HostName string   `json:"host_name"`
	TunnelIP string   `json:"tunnel_ip"`
	SSHUsers []string `json:"ssh_users"`
	Status   string   `json:"status"`
	LastSeen string   `json:"last_seen"`
}

// GetAuthorizedHosts 获取已授权主机列表
func (c *DesktopClient) GetAuthorizedHosts() ([]*HostInfo, error) {
	if !c.IsAuthenticated() {
		return nil, fmt.Errorf("未认证")
	}

	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()

	req := &pb.GetAuthorizedHostsRequest{
		DesktopId: c.desktopID,
	}

	resp, err := c.grpcClient.GetAuthorizedHosts(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("获取主机列表失败: %w", err)
	}

	hosts := make([]*HostInfo, 0, len(resp.Hosts))
	for _, h := range resp.Hosts {
		hosts = append(hosts, &HostInfo{
			HostID:   h.HostId,
			HostName: h.HostName,
			TunnelIP: h.TunnelIp,
			SSHUsers: h.SshUsers,
			Status:   h.Status,
			LastSeen: h.LastSeen,
		})
	}

	return hosts, nil
}

// GetHostServices 获取指定主机的服务列表
func (c *DesktopClient) GetHostServices(hostID string) ([]*pb.AuthorizedService, error) {
	if !c.IsAuthenticated() {
		return nil, fmt.Errorf("未认证")
	}

	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()

	req := &pb.GetHostServicesRequest{
		DesktopId: c.desktopID,
		HostId:    hostID,
	}

	resp, err := c.grpcClient.GetHostServices(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("获取主机服务失败: %w", err)
	}

	return resp.Services, nil
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

// GetMyDevices 获取我的设备列表
func (c *DesktopClient) GetMyDevices() ([]*DeviceInfo, error) {
	if !c.IsAuthenticated() {
		return nil, fmt.Errorf("未认证")
	}

	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()

	req := &pb.GetMyDevicesRequest{
		DesktopId: c.desktopID,
	}

	resp, err := c.grpcClient.GetMyDevices(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("获取设备列表失败: %w", err)
	}

	devices := make([]*DeviceInfo, 0, len(resp.Devices))
	for _, d := range resp.Devices {
		devices = append(devices, &DeviceInfo{
			DeviceToken: d.DeviceToken,
			DeviceName:  d.DeviceName,
			OS:          d.Os,
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

// OfflineDevice 设备下线
func (c *DesktopClient) OfflineDevice(deviceToken string) error {
	if !c.IsAuthenticated() {
		return fmt.Errorf("未认证")
	}

	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()

	req := &pb.OfflineDeviceRequest{
		DesktopId:   c.desktopID,
		DeviceToken: deviceToken,
	}

	resp, err := c.grpcClient.OfflineDevice(ctx, req)
	if err != nil {
		return fmt.Errorf("设备下线失败: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf(resp.Message)
	}

	return nil
}

// DeleteDevice 删除设备
func (c *DesktopClient) DeleteDevice(deviceToken string) error {
	if !c.IsAuthenticated() {
		return fmt.Errorf("未认证")
	}

	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()

	req := &pb.DeleteDeviceRequest{
		DesktopId:   c.desktopID,
		DeviceToken: deviceToken,
	}

	resp, err := c.grpcClient.DeleteDevice(ctx, req)
	if err != nil {
		return fmt.Errorf("删除设备失败: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf(resp.Message)
	}

	return nil
}

// ToggleFavorite 切换服务收藏状态
func (c *DesktopClient) ToggleFavorite(serviceID string) (bool, error) {
	c.mu.RLock()
	desktopID := c.desktopID
	c.mu.RUnlock()

	if desktopID == 0 {
		return false, fmt.Errorf("未认证")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.grpcClient.ToggleFavorite(ctx, &pb.ToggleFavoriteRequest{
		DesktopId: desktopID,
		ServiceId: serviceID,
	})
	if err != nil {
		return false, fmt.Errorf("切换收藏状态失败: %w", err)
	}

	if !resp.Success {
		return false, fmt.Errorf(resp.Message)
	}

	return resp.IsFavorite, nil
}

// GetFavoriteServices 获取收藏的服务列表
func (c *DesktopClient) GetFavoriteServices() ([]string, error) {
	c.mu.RLock()
	desktopID := c.desktopID
	c.mu.RUnlock()

	if desktopID == 0 {
		return nil, fmt.Errorf("未认证")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.grpcClient.GetFavoriteServices(ctx, &pb.GetFavoriteServicesRequest{
		DesktopId: desktopID,
	})
	if err != nil {
		return nil, fmt.Errorf("获取收藏列表失败: %w", err)
	}

	return resp.ServiceIds, nil
}
