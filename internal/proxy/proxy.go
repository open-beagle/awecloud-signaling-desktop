// Package proxy 提供本地 TCP 代理功能
// 在 VIP 地址上监听，将流量通过 tsnet 转发到远程 Agent
package proxy

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"sync"
	"time"
)

// DialFunc 通过 tsnet 拨号的函数签名
type DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// Target 代理目标信息
type Target struct {
	Domain     string // 域名（如 pg.yygl.beijing.beagle）
	VIP        string // 本地 VIP 地址（如 127.1.0.1）
	RemoteAddr string // 远程地址（Agent Tailscale IP:端口）
	Port       int    // 监听端口（与远程端口相同）
	TLS        bool   // 是否在本地做 TLS 终止（k8sapi 类型需要）
}

// entry 单个代理实例
type entry struct {
	target   Target
	listener net.Listener
	cancel   context.CancelFunc
	lastUsed time.Time
	mu       sync.Mutex
}

// Manager 本地代理管理器
// 管理多个 VIP:端口 → tsnet → Agent 的代理
type Manager struct {
	dial    DialFunc
	proxies map[string]*entry // key: "vip:port"
	mu      sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewManager 创建代理管理器
func NewManager(dial DialFunc) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		dial:    dial,
		proxies: make(map[string]*entry),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// StartProxy 启动一个本地代理
// 在 vip:port 上监听，转发到 remoteAddr
// 如果 target.TLS 为 true，使用自签证书做 TLS 终止（用于 k8sapi，kubectl 需要 HTTPS）
func (m *Manager) StartProxy(target Target) error {
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

	// 如果需要 TLS 终止，用自签证书包装 listener
	// kubectl 使用 --insecure-skip-tls-verify 跳过证书验证
	if target.TLS {
		tlsCert, err := generateSelfSignedCert()
		if err != nil {
			listener.Close()
			return fmt.Errorf("生成自签证书失败: %w", err)
		}
		listener = tls.NewListener(listener, &tls.Config{
			Certificates: []tls.Certificate{tlsCert},
		})
		log.Printf("[Proxy] TLS 终止已启用: %s", listenAddr)
	}

	ctx, cancel := context.WithCancel(m.ctx)
	e := &entry{
		target:   target,
		listener: listener,
		cancel:   cancel,
		lastUsed: time.Now(),
	}

	m.mu.Lock()
	m.proxies[key] = e
	m.mu.Unlock()

	m.wg.Add(1)
	go m.acceptLoop(ctx, e)

	log.Printf("[Proxy] 已启动: %s → %s (%s)", listenAddr, target.RemoteAddr, target.Domain)
	return nil
}

// StopProxy 停止一个代理
func (m *Manager) StopProxy(vip string, port int) {
	key := fmt.Sprintf("%s:%d", vip, port)

	m.mu.Lock()
	e, exists := m.proxies[key]
	if exists {
		delete(m.proxies, key)
	}
	m.mu.Unlock()

	if exists {
		e.cancel()
		e.listener.Close()
		log.Printf("[Proxy] 已停止: %s (%s)", key, e.target.Domain)
	}
}

// StopAll 停止所有代理
func (m *Manager) StopAll() {
	m.cancel()

	m.mu.Lock()
	for key, e := range m.proxies {
		e.listener.Close()
		delete(m.proxies, key)
	}
	m.mu.Unlock()

	m.wg.Wait()
	log.Printf("[Proxy] 所有代理已停止")
}

// GetStatus 获取所有代理状态
func (m *Manager) GetStatus() []Target {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]Target, 0, len(m.proxies))
	for _, e := range m.proxies {
		result = append(result, e.target)
	}
	return result
}

// Count 获取运行中的代理数量
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.proxies)
}

// acceptLoop 接受连接循环
func (m *Manager) acceptLoop(ctx context.Context, e *entry) {
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
				log.Printf("[Proxy] Accept 失败 (%s): %v", e.target.Domain, err)
				continue
			}
		}

		// 更新最后使用时间
		e.mu.Lock()
		e.lastUsed = time.Now()
		e.mu.Unlock()

		go m.handleConn(ctx, conn, e.target)
	}
}

// handleConn 处理单个连接
func (m *Manager) handleConn(ctx context.Context, clientConn net.Conn, target Target) {
	defer clientConn.Close()

	// 通过 tsnet 拨号到远程 Agent
	dialCtx, dialCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dialCancel()

	remoteConn, err := m.dial(dialCtx, "tcp", target.RemoteAddr)
	if err != nil {
		log.Printf("[Proxy] 连接远程失败 (%s → %s): %v", target.Domain, target.RemoteAddr, err)
		return
	}
	defer remoteConn.Close()

	// 双向转发，等待两个方向都完成
	done := make(chan struct{}, 2)
	go func() {
		io.Copy(remoteConn, clientConn)
		// 客户端读完，半关闭远程写方向
		if tc, ok := remoteConn.(interface{ CloseWrite() error }); ok {
			tc.CloseWrite()
		}
		done <- struct{}{}
	}()
	go func() {
		io.Copy(clientConn, remoteConn)
		// 远程读完，半关闭客户端写方向
		if tc, ok := clientConn.(interface{ CloseWrite() error }); ok {
			tc.CloseWrite()
		}
		done <- struct{}{}
	}()

	// 等待两个方向都完成
	select {
	case <-ctx.Done():
	case <-done:
		// 第一个方向完成，等待第二个
		select {
		case <-done:
		case <-ctx.Done():
		}
	}
}

// generateSelfSignedCert 生成自签 TLS 证书
// 用于 K8S API 代理的本地 TLS 终止
// kubectl 使用 --insecure-skip-tls-verify 跳过证书验证
func generateSelfSignedCert() (tls.Certificate, error) {
	// 生成 ECDSA 私钥
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("生成私钥失败: %w", err)
	}

	// 创建证书模板
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"AWECloud K8S API Proxy"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour), // 10 年有效期
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost", "*.beagle"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	// 自签证书
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("创建证书失败: %w", err)
	}

	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privateKey,
	}, nil
}
