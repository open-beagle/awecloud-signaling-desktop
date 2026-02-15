// Package proxy 提供本地 TCP 代理功能
// svc_proxy.go 实现 K8S Service 的 gRPC SVCProxy 代理
// 在 VIP 地址上监听 TCP，通过 tsnet 拨号到 Agent gRPC 端口，
// 建立 SVCProxy 双向流，桥接 TCP ↔ gRPC 数据
package proxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/open-beagle/awecloud-signaling-desktop/pkg/proto"
)

// SVCTarget K8S Service 代理目标信息
type SVCTarget struct {
	Domain       string // 域名（如 postgres.default.beijing.beagle）
	VIP          string // 本地 VIP 地址（如 127.1.0.1）
	Port         int    // 监听端口（与目标服务端口相同）
	AgentIP      string // Agent 的 Tailscale IP
	GRPCPort     int    // Agent SVCProxy gRPC 端口（默认 50051）
	Namespace    string // K8S 命名空间
	ServiceName  string // K8S Service 名称
	TargetPort   int    // K8S Service 目标端口
	EndpointName string // Endpoint 名称（非空时走 Endpoint 跳跃路径）
}

// svcEntry 单个 SVCProxy 代理实例
type svcEntry struct {
	target   SVCTarget
	listener net.Listener
	cancel   context.CancelFunc
}

// SVCProxyManager K8S Service gRPC 代理管理器
type SVCProxyManager struct {
	dial    DialFunc
	proxies map[string]*svcEntry // key: "vip:port"
	mu      sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewSVCProxyManager 创建 SVCProxy 代理管理器
func NewSVCProxyManager(dial DialFunc) *SVCProxyManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &SVCProxyManager{
		dial:    dial,
		proxies: make(map[string]*svcEntry),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// StartSVCProxy 启动一个 K8S Service gRPC 代理
func (m *SVCProxyManager) StartSVCProxy(target SVCTarget) error {
	key := fmt.Sprintf("%s:%d", target.VIP, target.Port)

	m.mu.Lock()
	if _, exists := m.proxies[key]; exists {
		m.mu.Unlock()
		return nil // 已存在，跳过
	}
	m.mu.Unlock()

	// 监听 VIP 地址
	listenAddr := fmt.Sprintf("%s:%d", target.VIP, target.Port)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("监听 %s 失败: %w", listenAddr, err)
	}

	ctx, cancel := context.WithCancel(m.ctx)
	e := &svcEntry{
		target:   target,
		listener: listener,
		cancel:   cancel,
	}

	m.mu.Lock()
	m.proxies[key] = e
	m.mu.Unlock()

	m.wg.Add(1)
	go m.acceptLoop(ctx, e)

	log.Printf("[SVCProxy] 已启动: %s → %s:%d (ns=%s, svc=%s, port=%d)",
		listenAddr, target.AgentIP, target.GRPCPort,
		target.Namespace, target.ServiceName, target.TargetPort)
	return nil
}

// StopAll 停止所有 SVCProxy 代理
func (m *SVCProxyManager) StopAll() {
	m.cancel()

	m.mu.Lock()
	for key, e := range m.proxies {
		e.listener.Close()
		delete(m.proxies, key)
	}
	m.mu.Unlock()

	m.wg.Wait()
	log.Printf("[SVCProxy] 所有代理已停止")
}

// Count 获取运行中的代理数量
func (m *SVCProxyManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.proxies)
}

// GetStatus 获取所有运行中的 SVCProxy 代理目标信息
func (m *SVCProxyManager) GetStatus() []SVCTarget {
	m.mu.RLock()
	defer m.mu.RUnlock()
	targets := make([]SVCTarget, 0, len(m.proxies))
	for _, e := range m.proxies {
		targets = append(targets, e.target)
	}
	return targets
}

// acceptLoop 接受连接循环
func (m *SVCProxyManager) acceptLoop(ctx context.Context, e *svcEntry) {
	defer m.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		conn, err := e.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				log.Printf("[SVCProxy] Accept 失败 (%s): %v", e.target.Domain, err)
				continue
			}
		}

		go m.handleConn(ctx, conn, e.target)
	}
}

// handleConn 处理单个 TCP 连接，桥接到 Agent gRPC SVCProxy
func (m *SVCProxyManager) handleConn(ctx context.Context, clientConn net.Conn, target SVCTarget) {
	defer clientConn.Close()

	// 1. 通过 tsnet 拨号到 Agent gRPC 端口
	grpcAddr := fmt.Sprintf("%s:%d", target.AgentIP, target.GRPCPort)
	grpcConn, err := grpc.NewClient(
		grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return m.dial(ctx, "tcp", addr)
		}),
	)
	if err != nil {
		log.Printf("[SVCProxy] gRPC 连接失败 (%s): %v", grpcAddr, err)
		return
	}
	defer grpcConn.Close()

	// 2. 创建 SVCProxy 客户端并建立双向流
	svcClient := pb.NewAgentServiceClient(grpcConn)
	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()

	stream, err := svcClient.SVCProxy(streamCtx)
	if err != nil {
		log.Printf("[SVCProxy] 建立流失败 (%s): %v", target.Domain, err)
		return
	}

	// 3. 发送首包（连接请求，携带 endpoint_name 用于 Endpoint 跳跃路径）
	if err := stream.Send(&pb.SVCProxyData{
		Namespace:    target.Namespace,
		ServiceName:  target.ServiceName,
		Port:         int32(target.TargetPort),
		IsConnect:    true,
		EndpointName: target.EndpointName,
	}); err != nil {
		log.Printf("[SVCProxy] 发送首包失败 (%s): %v", target.Domain, err)
		return
	}

	// 4. 等待 Agent 确认（检查是否有错误响应）
	// Agent 如果权限拒绝或 Service 未找到，会立即发送带 error 的消息
	// 这里用短超时检查首个响应
	firstRespCh := make(chan *pb.SVCProxyData, 1)
	firstErrCh := make(chan error, 1)
	go func() {
		resp, err := stream.Recv()
		if err != nil {
			firstErrCh <- err
			return
		}
		firstRespCh <- resp
	}()

	select {
	case resp := <-firstRespCh:
		if resp.Error != "" {
			log.Printf("[SVCProxy] Agent 拒绝连接 (%s): %s", target.Domain, resp.Error)
			return
		}
		// Agent 可能发送了数据，写入客户端
		if len(resp.Data) > 0 {
			clientConn.Write(resp.Data)
		}
		if resp.IsClose {
			return
		}
	case err := <-firstErrCh:
		log.Printf("[SVCProxy] 接收首响应失败 (%s): %v", target.Domain, err)
		return
	case <-time.After(10 * time.Second):
		// 超时说明 Agent 没有立即返回错误，连接正常建立
		// 继续桥接
	}

	log.Printf("[SVCProxy] 连接建立: %s → %s (ns=%s, svc=%s, port=%d)",
		clientConn.RemoteAddr(), grpcAddr, target.Namespace, target.ServiceName, target.TargetPort)

	// 5. 双向桥接（TCP ↔ gRPC stream）
	var wg sync.WaitGroup
	wg.Add(2)

	// TCP → gRPC
	go func() {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			n, err := clientConn.Read(buf)
			if n > 0 {
				if sendErr := stream.Send(&pb.SVCProxyData{Data: buf[:n]}); sendErr != nil {
					return
				}
			}
			if err != nil {
				// 发送关闭通知
				stream.Send(&pb.SVCProxyData{IsClose: true})
				stream.CloseSend()
				return
			}
		}
	}()

	// gRPC → TCP
	go func() {
		defer wg.Done()
		for {
			msg, err := stream.Recv()
			if err != nil {
				if err != io.EOF {
					log.Printf("[SVCProxy] gRPC 接收结束 (%s): %v", target.Domain, err)
				}
				return
			}
			if msg.Error != "" {
				log.Printf("[SVCProxy] Agent 错误 (%s): %s", target.Domain, msg.Error)
				return
			}
			if msg.IsClose {
				return
			}
			if len(msg.Data) > 0 {
				if _, err := clientConn.Write(msg.Data); err != nil {
					return
				}
			}
		}
	}()

	wg.Wait()
}
