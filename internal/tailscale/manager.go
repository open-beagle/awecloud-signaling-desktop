// Package tailscale 提供 Desktop 端 Tailscale 管理功能
package tailscale

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"

	"tailscale.com/tsnet"
)

// Manager 管理 Desktop 端 Tailscale 客户端
type Manager struct {
	tsServer *tsnet.Server

	tailscaleIP string
	connected   bool
	mutex       sync.RWMutex

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

// Connect 连接 Tailscale 网络
func (m *Manager) Connect(controlURL, authKey, hostname string) error {
	log.Printf("[Tailscale] Connecting to: %s", controlURL)

	// 确定状态存储目录
	stateDir := m.getStateDir()
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return fmt.Errorf("failed to create state dir: %w", err)
	}

	// 创建 tsnet.Server
	m.tsServer = &tsnet.Server{
		Hostname:   hostname,
		Dir:        stateDir,
		ControlURL: controlURL,
		AuthKey:    authKey,
		Ephemeral:  true, // Desktop 使用临时节点，断开后自动清理
	}

	// 启动 Tailscale
	status, err := m.tsServer.Up(m.ctx)
	if err != nil {
		return fmt.Errorf("failed to start Tailscale: %w", err)
	}

	// 获取 Tailscale IP
	if len(status.TailscaleIPs) > 0 {
		m.mutex.Lock()
		m.tailscaleIP = status.TailscaleIPs[0].String()
		m.connected = true
		m.mutex.Unlock()

		log.Printf("[Tailscale] Connected, IP: %s", m.tailscaleIP)
	} else {
		return fmt.Errorf("no Tailscale IP assigned")
	}

	return nil
}

// Disconnect 断开 Tailscale 连接
func (m *Manager) Disconnect() error {
	m.cancel()

	m.mutex.Lock()
	m.connected = false
	m.tailscaleIP = ""
	m.mutex.Unlock()

	if m.tsServer != nil {
		if err := m.tsServer.Close(); err != nil {
			log.Printf("[Tailscale] Close error: %v", err)
			return err
		}
		m.tsServer = nil
	}

	log.Printf("[Tailscale] Disconnected")
	return nil
}

// Dial 通过 Tailscale 网络拨号
func (m *Manager) Dial(ctx context.Context, network, addr string) (net.Conn, error) {
	if m.tsServer == nil {
		return nil, fmt.Errorf("Tailscale not connected")
	}
	return m.tsServer.Dial(ctx, network, addr)
}

// GetIP 获取 Tailscale IP
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
	// 根据操作系统选择目录
	var baseDir string

	if configDir, err := os.UserConfigDir(); err == nil {
		baseDir = configDir
	} else if homeDir, err := os.UserHomeDir(); err == nil {
		baseDir = filepath.Join(homeDir, ".config")
	} else {
		baseDir = "/tmp"
	}

	return filepath.Join(baseDir, "awecloud-desktop", "tailscale")
}
