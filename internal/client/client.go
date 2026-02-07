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
	heartbeatStopCh chan struct{} // 用于停止旧的 receiveHeartbeat goroutine

	// 数据流
	dataStream     pb.DesktopService_DataStreamClient
	dataStreamMutex sync.Mutex
	dataStreamStopCh chan struct{} // 用于停止旧的 receiveDataStream goroutine

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

	// 数据缓存（gRPC 断开时返回缓存数据）
	cachedHosts        []*HostInfo              // 主机列表缓存
	cachedHostServices map[string][]*pb.AuthorizedService // 主机服务缓存（key: hostID）
	cachedDevices      []*DeviceInfo            // 设备列表缓存
	cachedFavorites    []string                 // 收藏列表缓存
	cacheMutex         sync.RWMutex             // 保护所有缓存字段

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
		cachedHostServices: make(map[string][]*pb.AuthorizedService),
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

	// 停止心跳 goroutine
	c.heartbeatMutex.Lock()
	if c.heartbeatStopCh != nil {
		close(c.heartbeatStopCh)
		c.heartbeatStopCh = nil
	}
	c.heartbeatMutex.Unlock()

	// 停止数据流 goroutine
	c.dataStreamMutex.Lock()
	if c.dataStreamStopCh != nil {
		close(c.dataStreamStopCh)
		c.dataStreamStopCh = nil
	}
	c.dataStreamMutex.Unlock()

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
	// 检查 ctx 是否已取消（客户端正在停止）
	select {
	case <-c.ctx.Done():
		return fmt.Errorf("client stopped")
	default:
	}

	c.heartbeatMutex.Lock()
	defer c.heartbeatMutex.Unlock()

	// 更新隧道状态
	if tunnelIP != "" {
		c.tunnelMutex.Lock()
		c.tunnelIP = tunnelIP
		c.tunnelConnected = tunnelConnected
		c.tunnelMutex.Unlock()
	} else if c.getTunnelStatus != nil {
		ip, connected := c.getTunnelStatus()
		if ip != "" {
			c.tunnelMutex.Lock()
			c.tunnelIP = ip
			c.tunnelConnected = connected
			c.tunnelMutex.Unlock()
		}
	}

	// 停止旧的 receiveHeartbeat/sendHeartbeat goroutine
	if c.heartbeatStopCh != nil {
		close(c.heartbeatStopCh)
	}
	c.heartbeatStopCh = make(chan struct{})

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

	// 启动接收和发送 goroutine（使用 stopCh 控制生命周期）
	stopCh := c.heartbeatStopCh
	go c.receiveHeartbeat(stopCh)
	go c.sendHeartbeat(stopCh)

	return nil
}

// receiveHeartbeat 接收心跳响应（通过 stopCh 控制退出）
func (c *DesktopClient) receiveHeartbeat(stopCh <-chan struct{}) {
	backoff := time.Second * 5
	maxBackoff := time.Minute * 2

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-stopCh:
			return
		default:
		}

		c.heartbeatMutex.Lock()
		stream := c.heartbeatStream
		c.heartbeatMutex.Unlock()

		if stream == nil {
			select {
			case <-time.After(time.Second):
			case <-stopCh:
				return
			case <-c.ctx.Done():
				return
			}
			continue
		}

		_, err := stream.Recv()
		if err != nil {
			log.Printf("[DesktopClient] Heartbeat receive error: %v", err)
			c.setGRPCConnected(false)

			// 检查是否已被停止
			select {
			case <-stopCh:
				return
			case <-c.ctx.Done():
				return
			default:
			}

			log.Printf("[DesktopClient] Will retry in %v", backoff)
			select {
			case <-time.After(backoff):
			case <-c.ctx.Done():
				return
			case <-stopCh:
				return
			}

			backoff = min(backoff*2, maxBackoff)

			// reconnect 会调用 startHeartbeat，启动新 goroutine 并 close 当前 stopCh
			if c.IsAuthenticated() {
				if err := c.reconnect(); err != nil {
					log.Printf("[DesktopClient] Reconnect failed: %v", err)
				} else {
					log.Printf("[DesktopClient] Reconnected successfully")
					return // 新 goroutine 已启动，当前退出
				}
			}
			continue
		}

		backoff = time.Second * 5
		c.setGRPCConnected(true)

		log.Printf("[DEBUG] [DesktopClient] Heartbeat received")
	}
}

// reconnect 重连：重新 Authenticate 并启动心跳
func (c *DesktopClient) reconnect() error {
	c.mu.RLock()
	desktopID := c.desktopID
	secret := c.secret
	c.mu.RUnlock()

	if desktopID == 0 || secret == "" {
		return fmt.Errorf("no credentials")
	}

	// 重新 Authenticate
	log.Printf("[DesktopClient] Re-authenticating: desktopID=%d", desktopID)
	_, err := c.Authenticate(desktopID, secret)
	if err != nil {
		log.Printf("[DesktopClient] Re-authenticate failed: %v", err)
		// 凭证无效，触发重连回调（通知 App 层）
		if c.onReconnectNeeded != nil {
			if cbErr := c.onReconnectNeeded(); cbErr != nil {
				log.Printf("[DesktopClient] Reconnect callback failed: %v", cbErr)
			}
		}
		return err
	}

	return nil
}

// sendHeartbeat 发送心跳（通过 stopCh 控制退出）
func (c *DesktopClient) sendHeartbeat(stopCh <-chan struct{}) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-stopCh:
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
		// gRPC 失败，返回缓存数据
		log.Printf("[DesktopClient] 获取主机列表失败，使用缓存: %v", err)
		c.cacheMutex.RLock()
		cached := c.cachedHosts
		c.cacheMutex.RUnlock()
		if cached != nil {
			return cached, nil
		}
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

	// 更新缓存
	c.cacheMutex.Lock()
	c.cachedHosts = hosts
	c.cacheMutex.Unlock()

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
		// gRPC 失败，返回缓存数据
		log.Printf("[DesktopClient] 获取主机服务失败，使用缓存: %v", err)
		c.cacheMutex.RLock()
		cached := c.cachedHostServices[hostID]
		c.cacheMutex.RUnlock()
		if cached != nil {
			return cached, nil
		}
		return nil, fmt.Errorf("获取主机服务失败: %w", err)
	}

	// 更新缓存
	c.cacheMutex.Lock()
	c.cachedHostServices[hostID] = resp.Services
	c.cacheMutex.Unlock()

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
	IP          string `json:"ip"`
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
		// gRPC 失败，返回缓存数据
		log.Printf("[DesktopClient] 获取设备列表失败，使用缓存: %v", err)
		c.cacheMutex.RLock()
		cached := c.cachedDevices
		c.cacheMutex.RUnlock()
		if cached != nil {
			return cached, nil
		}
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
			IP:          d.Ip,
		})
	}

	// 更新缓存
	c.cacheMutex.Lock()
	c.cachedDevices = devices
	c.cacheMutex.Unlock()

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
		// gRPC 失败，返回缓存数据
		log.Printf("[DesktopClient] 获取收藏列表失败，使用缓存: %v", err)
		c.cacheMutex.RLock()
		cached := c.cachedFavorites
		c.cacheMutex.RUnlock()
		if cached != nil {
			return cached, nil
		}
		return nil, fmt.Errorf("获取收藏列表失败: %w", err)
	}

	// 更新缓存
	c.cacheMutex.Lock()
	c.cachedFavorites = resp.ServiceIds
	c.cacheMutex.Unlock()

	return resp.ServiceIds, nil
}

// LoginResultFromGRPC gRPC 登录结果
type LoginResultFromGRPC struct {
	Success     bool
	Message     string
	DesktopID   uint64
	DeviceToken string
	AuthKey     string
	ServerURL   string
	Username    string
}

// WaitForLoginResult 通过 gRPC 双向流等待登录结果
// 此方法建立 gRPC 连接，发送 sessionID，等待 Server 推送登录结果
func (c *DesktopClient) WaitForLoginResult(sessionID, deviceFingerprint string) (*LoginResultFromGRPC, error) {
	log.Printf("[Client] WaitForLoginResult: sessionID=%s", sessionID)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// 建立双向流
	stream, err := c.grpcClient.WaitForLoginResult(ctx)
	if err != nil {
		log.Printf("[Client] Failed to create WaitForLoginResult stream: %v", err)
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}

	// 发送请求
	req := &pb.WaitForLoginResultRequest{
		SessionId:         sessionID,
		DeviceFingerprint: deviceFingerprint,
	}

	if err := stream.Send(req); err != nil {
		log.Printf("[Client] Failed to send WaitForLoginResult request: %v", err)
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	log.Printf("[Client] Sent WaitForLoginResult request, waiting for response...")

	// 接收响应
	resp, err := stream.Recv()
	if err != nil {
		log.Printf("[Client] Failed to receive WaitForLoginResult response: %v", err)
		return nil, fmt.Errorf("failed to receive response: %w", err)
	}

	log.Printf("[Client] Received WaitForLoginResult response: status=%v, message=%s", resp.Status, resp.Message)

	// 检查状态
	if resp.Status != pb.WaitForLoginResultStatus_WAIT_FOR_LOGIN_RESULT_STATUS_SUCCESS {
		return &LoginResultFromGRPC{
			Success: false,
			Message: resp.Message,
		}, nil
	}

	// 登录成功
	result := &LoginResultFromGRPC{
		Success:     true,
		Message:     resp.Message,
		DesktopID:   resp.DesktopId,
		DeviceToken: resp.DeviceToken,
		AuthKey:     resp.AuthKey,
		ServerURL:   resp.ServerUrl,
		Username:    resp.Username,
	}

	log.Printf("[Client] Login successful: desktopID=%d, username=%s", result.DesktopID, result.Username)

	return result, nil
}

// Logout 调用 Server gRPC 注销（安全离场）
func (c *DesktopClient) Logout() error {
	c.mu.RLock()
	desktopID := c.desktopID
	c.mu.RUnlock()

	if desktopID == 0 {
		return nil // 未认证，无需注销
	}

	// 带超时调用，最多等 5 秒
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.grpcClient.Logout(ctx, &pb.DesktopLogoutRequest{
		DesktopId: desktopID,
	})
	if err != nil {
		log.Printf("[DesktopClient] Logout gRPC 调用失败（忽略）: %v", err)
		return nil // 静默忽略，继续本地清理
	}

	log.Printf("[DesktopClient] Logout 响应: success=%v, message=%s", resp.Success, resp.Message)
	return nil
}

// startDataStream 启动数据流
func (c *DesktopClient) startDataStream() error {
	// 检查 ctx 是否已取消（客户端正在停止）
	select {
	case <-c.ctx.Done():
		return fmt.Errorf("client stopped")
	default:
	}

	c.dataStreamMutex.Lock()
	defer c.dataStreamMutex.Unlock()

	// 停止旧的 receiveDataStream goroutine
	if c.dataStreamStopCh != nil {
		close(c.dataStreamStopCh)
	}
	c.dataStreamStopCh = make(chan struct{})

	// 如果已有数据流，先关闭
	if c.dataStream != nil {
		c.dataStream.CloseSend()
		c.dataStream = nil
	}

	// 创建数据流
	stream, err := c.grpcClient.DataStream(c.ctx)
	if err != nil {
		return fmt.Errorf("failed to create data stream: %w", err)
	}

	c.dataStream = stream

	// 发送首条消息，携带 desktop_id
	req := &pb.DesktopDataRequest{
		DesktopId:   c.desktopID,
		RefreshType: pb.DesktopDataType_DESKTOP_DATA_TYPE_ALL,
	}
	if err := stream.Send(req); err != nil {
		return fmt.Errorf("failed to send initial data request: %w", err)
	}

	log.Printf("[DesktopClient] DataStream started")

	// 启动接收 goroutine
	stopCh := c.dataStreamStopCh
	go c.receiveDataStream(stopCh)

	return nil
}

// receiveDataStream 接收数据流推送（通过 stopCh 控制退出）
func (c *DesktopClient) receiveDataStream(stopCh <-chan struct{}) {
	backoff := time.Second * 5
	maxBackoff := time.Minute * 2

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-stopCh:
			return
		default:
		}

		c.dataStreamMutex.Lock()
		stream := c.dataStream
		c.dataStreamMutex.Unlock()

		if stream == nil {
			select {
			case <-time.After(time.Second):
			case <-stopCh:
				return
			case <-c.ctx.Done():
				return
			}
			continue
		}

		resp, err := stream.Recv()
		if err != nil {
			log.Printf("[DesktopClient] DataStream receive error: %v", err)

			// 检查是否已被停止
			select {
			case <-stopCh:
				return
			case <-c.ctx.Done():
				return
			default:
			}

			log.Printf("[DesktopClient] DataStream will retry in %v", backoff)
			select {
			case <-time.After(backoff):
			case <-c.ctx.Done():
				return
			case <-stopCh:
				return
			}

			backoff = min(backoff*2, maxBackoff)

			// 尝试重新建立数据流
			if c.IsAuthenticated() {
				if err := c.startDataStream(); err != nil {
					log.Printf("[DesktopClient] DataStream reconnect failed: %v", err)
				} else {
					log.Printf("[DesktopClient] DataStream reconnected")
					return // 新 goroutine 已启动，当前退出
				}
			}
			continue
		}

		backoff = time.Second * 5

		// 根据数据类型更新缓存
		c.handleDataStreamResponse(resp)
	}
}

// handleDataStreamResponse 处理数据流推送，更新本地缓存
func (c *DesktopClient) handleDataStreamResponse(resp *pb.DesktopDataResponse) {
	switch resp.Type {
	case pb.DesktopDataType_DESKTOP_DATA_TYPE_ALL:
		// 全量更新
		c.updateServicesCache(resp.Services)
		c.updateHostsCache(resp.Hosts)
		c.updateDevicesCache(resp.Devices)
		c.updateFavoritesCache(resp.FavoriteServiceIds)
		log.Printf("[DesktopClient] DataStream ALL: services=%d, hosts=%d, devices=%d, favorites=%d",
			len(resp.Services), len(resp.Hosts), len(resp.Devices), len(resp.FavoriteServiceIds))

	case pb.DesktopDataType_DESKTOP_DATA_TYPE_SERVICES:
		c.updateServicesCache(resp.Services)
		log.Printf("[DesktopClient] DataStream SERVICES: %d", len(resp.Services))

	case pb.DesktopDataType_DESKTOP_DATA_TYPE_HOSTS:
		c.updateHostsCache(resp.Hosts)
		log.Printf("[DesktopClient] DataStream HOSTS: %d", len(resp.Hosts))

	case pb.DesktopDataType_DESKTOP_DATA_TYPE_DEVICES:
		c.updateDevicesCache(resp.Devices)
		log.Printf("[DesktopClient] DataStream DEVICES: %d", len(resp.Devices))

	case pb.DesktopDataType_DESKTOP_DATA_TYPE_FAVORITES:
		c.updateFavoritesCache(resp.FavoriteServiceIds)
		log.Printf("[DesktopClient] DataStream FAVORITES: %d", len(resp.FavoriteServiceIds))
	}
}

// updateServicesCache 更新服务列表缓存
func (c *DesktopClient) updateServicesCache(services []*pb.AuthorizedService) {
	c.servicesMutex.Lock()
	c.authorizedServices = services
	c.servicesMutex.Unlock()
}

// updateHostsCache 更新主机列表缓存
func (c *DesktopClient) updateHostsCache(hosts []*pb.AuthorizedHost) {
	if hosts == nil {
		return
	}
	result := make([]*HostInfo, 0, len(hosts))
	for _, h := range hosts {
		result = append(result, &HostInfo{
			HostID:   h.HostId,
			HostName: h.HostName,
			TunnelIP: h.TunnelIp,
			SSHUsers: h.SshUsers,
			Status:   h.Status,
			LastSeen: h.LastSeen,
		})
	}
	c.cacheMutex.Lock()
	c.cachedHosts = result
	c.cacheMutex.Unlock()
}

// updateDevicesCache 更新设备列表缓存
func (c *DesktopClient) updateDevicesCache(devices []*pb.DeviceInfo) {
	if devices == nil {
		return
	}
	result := make([]*DeviceInfo, 0, len(devices))
	for _, d := range devices {
		result = append(result, &DeviceInfo{
			DeviceToken: d.DeviceToken,
			DeviceName:  d.DeviceName,
			OS:          d.Os,
			Arch:        d.Arch,
			Hostname:    d.Hostname,
			Status:      d.Status,
			LastUsedAt:  d.LastUsedAt,
			CreatedAt:   d.CreatedAt,
			IsCurrent:   d.IsCurrent,
			IP:          d.Ip,
		})
	}
	c.cacheMutex.Lock()
	c.cachedDevices = result
	c.cacheMutex.Unlock()
}

// updateFavoritesCache 更新收藏列表缓存
func (c *DesktopClient) updateFavoritesCache(favoriteIDs []string) {
	c.cacheMutex.Lock()
	c.cachedFavorites = favoriteIDs
	c.cacheMutex.Unlock()
}
