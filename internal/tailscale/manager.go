// Package tailscale 提供 Desktop 端 Tailscale 管理功能
// 使用 tsnet.Server 用户态模式，无需管理员/root 权限
package tailscale

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"tailscale.com/tsnet"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/config"
)

// Manager 管理 Desktop 端 Tailscale 客户端 (tsnet 用户态模式)
// 不需要管理员/root 权限，不创建 TUN 设备
type Manager struct {
	tsServer *tsnet.Server

	// 状态
	tailscaleIP string
	connected   bool
	mutex       sync.RWMutex

	// 生命周期
	ctx    context.Context
	cancel context.CancelFunc
}

// NewManager 创建 TailscaleManager
func NewManager() *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		ctx:    ctx,
		cancel: cancel,
	}
}

// Connect 连接隧道网络（tsnet 用户态模式）
func (m *Manager) Connect(controlURL, authKey, hostname string) error {
	log.Printf("[INFO] [Tunnel] 正在连接: %s (tsnet 用户态模式)", controlURL)

	// 初始化状态目录
	stateDir := m.getStateDir()
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return fmt.Errorf("创建状态目录失败: %w", err)
	}

	// 创建 tsnet.Server（用户态，无需 root 权限）
	m.tsServer = &tsnet.Server{
		Hostname:   hostname,
		Dir:        stateDir,
		ControlURL: controlURL,
		AuthKey:    authKey,
		Ephemeral:  false,          // Desktop 需要持久化节点
		Logf:       m.tailscaleLogf, // 自定义日志函数
	}

	log.Printf("[INFO] [Tunnel] 配置: ControlURL=%s, Hostname=%s, StateDir=%s", controlURL, hostname, stateDir)

	// 启动 tsnet（阻塞直到连接成功）
	status, err := m.tsServer.Up(m.ctx)
	if err != nil {
		m.tsServer = nil
		return fmt.Errorf("启动隧道失败: %w", err)
	}

	// 获取 Tailscale IP
	if len(status.TailscaleIPs) > 0 {
		m.mutex.Lock()
		m.tailscaleIP = status.TailscaleIPs[0].String()
		m.connected = true
		m.mutex.Unlock()

		log.Printf("[INFO] [Tunnel] 已连接，IP: %s", m.tailscaleIP)
	} else {
		return fmt.Errorf("未获取到隧道 IP")
	}

	// 启动状态监控
	go m.watchStatus()

	log.Printf("[INFO] [Tunnel] 引擎已启动（tsnet 用户态模式）")
	return nil
}

// watchStatus 监听状态变化
func (m *Manager) watchStatus() {
	if m.tsServer == nil {
		return
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	reconnectBackoff := time.Minute
	maxReconnectBackoff := 5 * time.Minute

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.updateStatus()

			// 检测断开，尝试重连
			if !m.IsConnected() {
				log.Printf("[WARN] [Tunnel] 连接断开，%v 后尝试重连", reconnectBackoff)
				select {
				case <-time.After(reconnectBackoff):
				case <-m.ctx.Done():
					return
				}
				if err := m.tryReconnect(); err != nil {
					log.Printf("[WARN] [Tunnel] 重连失败: %v", err)
					reconnectBackoff = min(reconnectBackoff*2, maxReconnectBackoff)
				} else {
					log.Printf("[INFO] [Tunnel] 重连成功")
					reconnectBackoff = time.Minute
				}
			} else {
				reconnectBackoff = time.Minute
			}
		}
	}
}

// updateStatus 更新连接状态
func (m *Manager) updateStatus() {
	if m.tsServer == nil {
		return
	}

	lc, err := m.tsServer.LocalClient()
	if err != nil {
		log.Printf("[DEBUG] [Tunnel] 获取 LocalClient 失败: %v", err)
		return
	}

	status, err := lc.Status(m.ctx)
	if err != nil {
		log.Printf("[DEBUG] [Tunnel] 获取状态失败: %v", err)
		return
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	wasConnected := m.connected
	m.connected = (status.BackendState == "Running")

	if len(status.TailscaleIPs) > 0 {
		m.tailscaleIP = status.TailscaleIPs[0].String()
	}

	if wasConnected != m.connected {
		if m.connected {
			log.Printf("[INFO] [Tunnel] 状态恢复: IP=%s", m.tailscaleIP)
		} else {
			log.Printf("[WARN] [Tunnel] 状态变化: %s", status.BackendState)
		}
	}
}

// tryReconnect 尝试重连
func (m *Manager) tryReconnect() error {
	if m.tsServer == nil {
		return fmt.Errorf("隧道未初始化")
	}

	lc, err := m.tsServer.LocalClient()
	if err != nil {
		return fmt.Errorf("获取 LocalClient 失败: %w", err)
	}

	status, err := lc.Status(m.ctx)
	if err != nil {
		return fmt.Errorf("获取状态失败: %w", err)
	}

	if status.BackendState == "Running" {
		m.mutex.Lock()
		m.connected = true
		m.mutex.Unlock()
		return nil
	}

	log.Printf("[INFO] [Tunnel] 后端状态: %s，尝试重新启动", status.BackendState)

	newStatus, err := m.tsServer.Up(m.ctx)
	if err != nil {
		return fmt.Errorf("重新启动隧道失败: %w", err)
	}

	if len(newStatus.TailscaleIPs) > 0 {
		m.mutex.Lock()
		m.tailscaleIP = newStatus.TailscaleIPs[0].String()
		m.connected = true
		m.mutex.Unlock()
		log.Printf("[INFO] [Tunnel] 重连成功，IP: %s", m.tailscaleIP)
	}

	return nil
}

// Disconnect 断开隧道连接
func (m *Manager) Disconnect() error {
	m.cancel()

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.tsServer != nil {
		if err := m.tsServer.Close(); err != nil {
			log.Printf("[WARN] [Tunnel] 关闭隧道失败: %v", err)
		}
		m.tsServer = nil
	}

	m.connected = false
	m.tailscaleIP = ""

	log.Printf("[INFO] [Tunnel] 已断开")
	return nil
}

// Dial 通过隧道网络拨号
func (m *Manager) Dial(ctx context.Context, network, addr string) (net.Conn, error) {
	if m.tsServer == nil {
		return nil, fmt.Errorf("隧道未启动")
	}
	return m.tsServer.Dial(ctx, network, addr)
}

// Listen 在隧道网络上监听端口
func (m *Manager) Listen(network, addr string) (net.Listener, error) {
	if m.tsServer == nil {
		return nil, fmt.Errorf("隧道未启动")
	}
	return m.tsServer.Listen(network, addr)
}

// GetIP 获取隧道 IP
func (m *Manager) GetIP() string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.tailscaleIP
}

// IsConnected 检查连接状态
func (m *Manager) IsConnected() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.connected
}

// getStateDir 获取状态存储目录
func (m *Manager) getStateDir() string {
	// 使用 config 包提供的统一目录
	if stateDir, err := config.GetTunnelStateDir(); err == nil {
		return stateDir
	}

	// 回退方案
	var baseDir string
	if configDir, err := os.UserConfigDir(); err == nil {
		baseDir = configDir
	} else if homeDir, err := os.UserHomeDir(); err == nil {
		baseDir = filepath.Join(homeDir, ".config")
	} else {
		baseDir = "/tmp"
	}
	return filepath.Join(baseDir, "signaling-desktop", "tunnel")
}

// tailscaleLogf 自定义日志函数，过滤 tailscale 内部日志
func (m *Manager) tailscaleLogf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)

	// 过滤掉过于频繁的 debug 日志
	if shouldFilterLog(msg) {
		return
	}

	// 替换 Tailscale 为 Tunnel
	msg = strings.ReplaceAll(msg, "Tailscale", "Tunnel")
	msg = strings.ReplaceAll(msg, "tailscale", "tunnel")
	log.Printf("[Tunnel] %s", msg)
}
