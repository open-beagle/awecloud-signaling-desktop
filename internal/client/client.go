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

	// 心跳流
	heartbeatStream pb.DesktopService_HeartbeatClient
	heartbeatMutex  sync.Mutex

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
	return c.desktopID > 0 && c.secret != ""
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

	// 如果已有心跳流，先关闭
	if c.heartbeatStream != nil {
		c.heartbeatStream.CloseSend()
		c.heartbeatStream = nil
	}

	// 创建心跳流
	stream, err := c.grpcClient.Heartbeat(c.ctx)
	if err != nil {
		return fmt.Errorf("failed to create heartbeat stream: %w", err)
	}

	c.heartbeatStream = stream

	// 发送首次心跳
	req := &pb.DesktopHeartbeatRequest{
		DesktopId:       c.desktopID,
		TunnelIp:        tunnelIP,
		TunnelConnected: tunnelConnected,
	}

	if err := stream.Send(req); err != nil {
		return fmt.Errorf("failed to send initial heartbeat: %w", err)
	}

	log.Printf("[DesktopClient] Heartbeat started")

	// 启动接收 goroutine
	go c.receiveHeartbeat()

	// 启动发送 goroutine
	go c.sendHeartbeat(tunnelIP, tunnelConnected)

	return nil
}

// receiveHeartbeat 接收心跳响应
func (c *DesktopClient) receiveHeartbeat() {
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
				time.Sleep(time.Second * 5)
				continue
			}

			// 更新已授权服务列表
			c.servicesMutex.Lock()
			c.authorizedServices = resp.AuthorizedServices
			c.servicesMutex.Unlock()

			log.Printf("[DEBUG] [DesktopClient] Heartbeat received, authorized services: %d", len(resp.AuthorizedServices))
		}
	}
}

// sendHeartbeat 发送心跳
func (c *DesktopClient) sendHeartbeat(tunnelIP string, tunnelConnected bool) {
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
