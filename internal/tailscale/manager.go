// Package tailscale 提供 Desktop 端 Tailscale 管理功能
package tailscale

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"tailscale.com/control/controlclient"
	"tailscale.com/health"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnlocal"
	"tailscale.com/ipn/store"
	"tailscale.com/net/netmon"
	"tailscale.com/net/tstun"
	"tailscale.com/tsd"
	"tailscale.com/types/logger"
	"tailscale.com/types/logid"
	"tailscale.com/util/eventbus"
	"tailscale.com/wgengine"
	"tailscale.com/wgengine/router"
)

// Manager 管理 Desktop 端 Tailscale 客户端 (System-Level VPN)
type Manager struct {
	lb *ipnlocal.LocalBackend

	tailscaleIP string
	connected   bool
	mutex       sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc

	// 内部状态
	backendLogID string
}

// NewManager 创建 TailscaleManager
func NewManager() *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		ctx:    ctx,
		cancel: cancel,
	}
}

// Connect 连接隧道网络
func (m *Manager) Connect(controlURL, authKey, hostname string) error {
	log.Printf("[Tunnel] Connecting to: %s (System VPN Mode)", controlURL)

	// 1. 检查权限
	if !isElevated() {
		return fmt.Errorf("requires administrator/root privileges")
	}

	stateDir := m.getStateDir()
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return fmt.Errorf("failed to create state dir: %w", err)
	}

	// 定义日志函数
	logf := logger.Logf(func(format string, args ...any) {
		log.Printf(format, args...)
	})

	// 2. 初始化核心依赖
	sys := tsd.NewSystem()

	eb := sys.Bus.Get()
	if eb == nil {
		eb = eventbus.New()
		sys.Bus.Set(eb)
	}

	// HealthTracker 需要 Bus
	ht := health.NewTracker(eb)

	// 3. 初始化 Store
	storePath := filepath.Join(stateDir, "tailscaled.state")
	fstore, err := store.New(logf, storePath)
	if err != nil {
		return fmt.Errorf("failed to create store: %w", err)
	}
	sys.Set(fstore)

	// 4. 创建 Network Monitor
	// Monitor v1.92 requires EventBus
	mon, err := netmon.New(eb, logf)
	if err != nil {
		return fmt.Errorf("failed to create netmon: %w", err)
	}
	// Start takes no args in v1.92
	mon.Start()
	sys.Set(mon)

	// 5. 创建 TUN 设备 (Wintun)
	// Rename to avoid conflicts
	tunName := "Signal-Tun"

	if runtime.GOOS == "windows" {
		// Explicitly pre-load wintun.dll to ensure we use the correct one
		// from the current directory, rather than any system version.
		dllName := "wintun.dll"
		absPath, _ := filepath.Abs(dllName)
		if _, err := os.Stat(absPath); err == nil {
			log.Printf("[Wintun] Pre-loading DLL from: %s", absPath)
			// We use syscall to load it into the process address space.
			// This prevents 'invalid memory address' panics caused by ABI mismatch
			// with older wintun.dll versions that might be in PATH.
			handle, err := syscall.LoadDLL(absPath)
			if err != nil {
				log.Printf("[Wintun] Failed to pre-load DLL: %v", err)
			} else {
				log.Printf("[Wintun] Pre-load successful (Handle: %v)", handle)
			}
		} else {
			log.Printf("[Wintun] Warning: wintun.dll not found at %s", absPath)
		}
	}

	tunDev, _, err := tstun.New(logf, tunName)
	if err != nil {
		mon.Close()
		return fmt.Errorf("failed to create TUN device: %w", err)
	}

	// 6. 创建 Router
	// New signature: (logf, dev, netmon, health, bus)
	// Compiler confirmed 5 arguments are required.
	r, err := router.New(logf, tunDev, mon, ht, eb)
	if err != nil {
		tunDev.Close()
		mon.Close()
		return fmt.Errorf("failed to create router: %w", err)
	}

	// 7. 创建 Engine
	e, err := wgengine.NewUserspaceEngine(logf, wgengine.Config{
		Tun:           tunDev,
		Router:        r,
		HealthTracker: ht,
		ListenPort:    41641,
	})
	if err != nil {
		r.Close()
		tunDev.Close()
		mon.Close()
		return fmt.Errorf("failed to create engine: %w", err)
	}
	sys.Set(e) // 关键：注入 Engine

	// 8. 创建 LocalBackend
	var pubID logid.PublicID

	lb, err := ipnlocal.NewLocalBackend(logf, pubID, sys, controlclient.LoginFlags(0))
	if err != nil {
		e.Close()
		return fmt.Errorf("failed to create local backend: %w", err)
	}
	m.lb = lb

	// 9. 启动流程
	opts := ipn.Options{
		AuthKey: authKey,
	}
	if err := lb.Start(opts); err != nil {
		return fmt.Errorf("failed to start backend: %w", err)
	}

	// 10. 应用配置
	prefs := ipn.NewPrefs()
	prefs.ControlURL = controlURL
	prefs.Hostname = hostname
	prefs.WantRunning = true

	if _, err := lb.EditPrefs(&ipn.MaskedPrefs{
		Prefs:          *prefs,
		ControlURLSet:  true,
		HostnameSet:    true,
		WantRunningSet: true,
	}); err != nil {
		return fmt.Errorf("failed to set prefs: %w", err)
	}

	// 11. 触发登录
	lb.StartLoginInteractive(m.ctx)

	go m.watchStatus()

	log.Printf("[Tunnel] System VPN Engine started. Please check logs/dashboard.")
	return nil
}

// 监听状态变化
func (m *Manager) watchStatus() {
	if m.lb == nil {
		return
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			st := m.lb.Status()
			ip := ""
			connected := (st.BackendState == "Running")

			if len(st.TailscaleIPs) > 0 {
				ip = st.TailscaleIPs[0].String()
			}

			m.mutex.Lock()
			changed := (m.connected != connected) || (m.tailscaleIP != ip)
			m.tailscaleIP = ip
			m.connected = connected
			m.mutex.Unlock()

			if changed {
				log.Printf("[Tunnel] Status update: State=%s, Connected=%v, IP=%s", st.BackendState, connected, ip)
			}
		}
	}
}

// 辅助：检查权限
func isElevated() bool {
	if runtime.GOOS == "windows" {
		f, err := os.Open("\\\\.\\PHYSICALDRIVE0")
		if err == nil {
			f.Close()
			return true
		}
		return false
	}
	return os.Geteuid() == 0
}

// Disconnect 断开隧道连接
func (m *Manager) Disconnect() error {
	m.cancel()

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.lb != nil {
		m.lb.Shutdown()
		m.lb = nil
	}

	m.connected = false
	m.tailscaleIP = ""

	log.Printf("[Tunnel] Disconnected")
	return nil
}

// Dial 通过隧道网络拨号
func (m *Manager) Dial(ctx context.Context, network, addr string) (net.Conn, error) {
	// 系统级 VPN 模式下，直接使用系统拨号器
	var d net.Dialer
	return d.DialContext(ctx, network, addr)
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
	var baseDir string
	if configDir, err := os.UserConfigDir(); err == nil {
		baseDir = configDir
	} else if homeDir, err := os.UserHomeDir(); err == nil {
		baseDir = filepath.Join(homeDir, ".config")
	} else {
		baseDir = "/tmp"
	}
	return filepath.Join(baseDir, "signal-desktop", "tailscale")
}
