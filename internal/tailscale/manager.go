// Package tailscale 提供 Desktop 端 Tailscale 管理功能
// 实现系统级 VPN，支持 Windows/Linux/macOS
package tailscale

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/config"
	"github.com/tailscale/wireguard-go/tun"
	"tailscale.com/control/controlclient"
	"tailscale.com/health"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnlocal"
	"tailscale.com/ipn/store"
	"tailscale.com/net/netmon"
	"tailscale.com/net/tsdial"
	"tailscale.com/net/tstun"
	"tailscale.com/tsd"
	"tailscale.com/types/logger"
	"tailscale.com/types/logid"
	"tailscale.com/util/eventbus"
	"tailscale.com/util/usermetric"
	"tailscale.com/wgengine"
	"tailscale.com/wgengine/router"

	// 导入 osrouter 包以注册平台特定的 router 实现
	_ "tailscale.com/wgengine/router/osrouter"
)

// Manager 管理 Desktop 端 Tailscale 客户端 (System-Level VPN)
// 支持 Windows/Linux/macOS 三平台
type Manager struct {
	lb *ipnlocal.LocalBackend

	// 状态
	tailscaleIP string
	connected   bool
	mutex       sync.RWMutex

	// 生命周期
	ctx    context.Context
	cancel context.CancelFunc

	// 资源清理
	cleanup []func()
}

// NewManager 创建 TailscaleManager
func NewManager() *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		ctx:     ctx,
		cancel:  cancel,
		cleanup: make([]func(), 0),
	}
}

// Connect 连接隧道网络
func (m *Manager) Connect(controlURL, authKey, hostname string) error {
	log.Printf("[INFO] [Tunnel] 正在连接: %s", controlURL)

	// 1. 检查权限
	if !IsElevated() {
		return fmt.Errorf("需要管理员/root 权限")
	}

	// 2. 平台特定初始化（Windows 预加载 Wintun）
	if err := PlatformInit(); err != nil {
		return fmt.Errorf("平台初始化失败: %w", err)
	}

	// 3. 初始化状态目录
	stateDir := m.getStateDir()
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return fmt.Errorf("创建状态目录失败: %w", err)
	}

	// 4. 定义日志函数（过滤 tailscale 内部日志，只打印重要信息）
	logf := logger.Logf(func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		// 过滤掉过于频繁的 debug 日志
		if shouldFilterLog(msg) {
			return
		}
		// 替换 Tailscale 为 Tunnel
		msg = strings.ReplaceAll(msg, "Tailscale", "Tunnel")
		msg = strings.ReplaceAll(msg, "tailscale", "tunnel")
		log.Printf("[Tunnel] %s", msg)
	})

	// 5. 初始化核心依赖 (tsd.System)
	sys := tsd.NewSystem()

	// EventBus
	eb := sys.Bus.Get()
	if eb == nil {
		eb = eventbus.New()
		sys.Bus.Set(eb)
	}

	// HealthTracker (由 NewTracker 内部设置到 sys)
	ht := health.NewTracker(eb)

	// Store (状态持久化)
	storePath := filepath.Join(stateDir, "tailscaled.state")
	fstore, err := store.New(logf, storePath)
	if err != nil {
		return fmt.Errorf("创建 Store 失败: %w", err)
	}
	sys.Set(fstore)

	// NetMon (网络监控)
	mon, err := netmon.New(eb, logf)
	if err != nil {
		return fmt.Errorf("创建 NetMon 失败: %w", err)
	}
	mon.Start()
	sys.Set(mon)
	m.cleanup = append(m.cleanup, func() { mon.Close() })

	// Dialer (新版 tailscale 必须，需要 NetMon)
	dialer := &tsdial.Dialer{Logf: logf}
	dialer.SetNetMon(mon)
	sys.Set(dialer)

	// Metrics (新版 tailscale 必须)
	metrics := new(usermetric.Registry)

	// 6. 创建 TUN 设备（Windows 使用自定义逻辑支持复用）
	var tunDev tun.Device
	var tunDevName string
	if runtime.GOOS == "windows" {
		tunDev, tunDevName, err = CreateTUN()
	} else {
		tunName := GetTunName()
		tunDev, tunDevName, err = tstun.New(logf, tunName)
	}
	if err != nil {
		m.cleanupAll()
		return fmt.Errorf("创建 TUN 设备失败: %w", err)
	}
	log.Printf("[DEBUG] [Tunnel] TUN 设备已创建: %s", tunDevName)
	m.cleanup = append(m.cleanup, func() { tunDev.Close() })

	// 7. 创建 Router
	r, err := router.New(logf, tunDev, mon, ht, eb)
	if err != nil {
		m.cleanupAll()
		return fmt.Errorf("创建 Router 失败: %w", err)
	}
	m.cleanup = append(m.cleanup, func() { r.Close() })

	// 8. 创建 WGEngine
	e, err := wgengine.NewUserspaceEngine(logf, wgengine.Config{
		Tun:           tunDev,
		Router:        r,
		Dialer:        dialer,
		NetMon:        mon,
		HealthTracker: ht,
		Metrics:       metrics,
		ListenPort:    41641,
		EventBus:      eb,
		SetSubsystem:  sys.Set, // 自动注册 magicsock 等组件到 sys
	})
	if err != nil {
		m.cleanupAll()
		return fmt.Errorf("创建 Engine 失败: %w", err)
	}
	sys.Set(e) // Engine 需要手动注册
	m.cleanup = append(m.cleanup, func() { e.Close() })

	// 9. 创建 LocalBackend
	var pubID logid.PublicID
	lb, err := ipnlocal.NewLocalBackend(logf, pubID, sys, controlclient.LoginFlags(0))
	if err != nil {
		m.cleanupAll()
		return fmt.Errorf("创建 LocalBackend 失败: %w", err)
	}
	m.lb = lb

	// 10. 构建初始配置（必须在 Start 之前设置 ControlURL）
	prefs := ipn.NewPrefs()
	prefs.ControlURL = controlURL
	prefs.Hostname = hostname
	prefs.WantRunning = true

	log.Printf("[INFO] [Tunnel] 配置: ControlURL=%s, Hostname=%s", controlURL, hostname)

	// 11. 启动（带初始配置）
	opts := ipn.Options{
		AuthKey:     authKey,
		UpdatePrefs: prefs,
	}
	if err := lb.Start(opts); err != nil {
		m.cleanupAll()
		return fmt.Errorf("启动 Backend 失败: %w", err)
	}

	// 12. 启动 TUN wrapper（必须在 lb.Start 之后）
	// wgengine 内部会用 tstun.Wrap 包装 TUN 设备并存到 sys.Tun
	if w, ok := sys.Tun.GetOK(); ok {
		w.Start()
	}

	// 13. 触发登录
	lb.StartLoginInteractive(m.ctx)

	// 14. 启动状态监控
	go m.watchStatus()

	log.Printf("[INFO] [Tunnel] 引擎已启动")
	return nil
}

// watchStatus 监听状态变化
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

			// 打印详细状态
			log.Printf("[DEBUG] [Tunnel] 状态: State=%s, IP=%s, AuthURL=%s", st.BackendState, ip, st.AuthURL)

			m.mutex.Lock()
			changed := (m.connected != connected) || (m.tailscaleIP != ip)
			m.tailscaleIP = ip
			m.connected = connected
			m.mutex.Unlock()

			if changed {
				if connected {
					log.Printf("[Tunnel] 已连接: IP=%s", ip)
				} else {
					log.Printf("[DEBUG] [Tunnel] 状态: %s", st.BackendState)
				}
			}
		}
	}
}

// cleanupAll 清理所有资源（逆序）
func (m *Manager) cleanupAll() {
	for i := len(m.cleanup) - 1; i >= 0; i-- {
		m.cleanup[i]()
	}
	m.cleanup = nil
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

	m.cleanupAll()

	m.connected = false
	m.tailscaleIP = ""

	log.Printf("[INFO] [Tunnel] 已断开")
	return nil
}

// Dial 通过隧道网络拨号
// 系统级 VPN 模式下，直接使用系统拨号器
func (m *Manager) Dial(ctx context.Context, network, addr string) (net.Conn, error) {
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

// IsElevated 检查是否有管理员权限
func IsElevated() bool {
	switch runtime.GOOS {
	case "windows":
		// Windows: 尝试打开物理磁盘来检测管理员权限
		f, err := os.Open("\\\\.\\PHYSICALDRIVE0")
		if err == nil {
			f.Close()
			return true
		}
		return false
	default:
		// Linux/macOS: 检查 euid 是否为 0
		return os.Geteuid() == 0
	}
}

// shouldFilterLog 判断是否应该过滤掉该日志
// 过滤掉过于频繁的 debug 日志，只保留重要信息
func shouldFilterLog(msg string) bool {
	// warming-up 是正常启动状态，不是错误，过滤掉
	if strings.Contains(msg, "warming-up") {
		return true
	}

	// health 日志特殊处理
	if strings.Contains(msg, "health(") {
		// ok 状态过滤掉
		if strings.Contains(msg, "): ok") {
			return true
		}
		// 真正的错误保留
		if strings.Contains(msg, "error:") {
			return false
		}
	}

	// 重要日志，不过滤
	importantPatterns := []string{
		"error",        // 错误
		"Error",        // 错误
		"failed",       // 失败
		"Failed",       // 失败
		"已连接",          // 连接成功
		"已断开",          // 断开
		"正在连接",         // 连接中
		"TUN 设备已创建",    // TUN 创建
		"引擎已启动",        // 引擎启动
		"active login", // 登录成功
	}

	for _, pattern := range importantPatterns {
		if strings.Contains(msg, pattern) {
			return false // 不过滤
		}
	}

	// 过滤掉的日志模式
	filterPatterns := []string{
		// 网络监控事件（太频繁）
		"monitor: got windows change event",
		"monitor: [unexpected]",
		"monitor: old:",
		"monitor: new:",
		// 详细的网络状态 JSON
		"InterfaceIPs",
		"HardwareAddr",
		// WireGuard 内部日志
		"wg: [v2]",
		// 控制协议详细日志
		"control: [v1]",
		"control: [v2]",
		"control: [vJSON]",
		// 网络检查
		"netcheck: [v1] report:",
		// 路由器防火墙详细日志
		"router: firewall:",
		"router: monitorDefaultRoutes",
		// DNS 配置详细日志
		"dns: Set:",
		"dns: Resolvercfg:",
		"dns: OScfg:",
		// 其他详细日志
		"[v1] netmap",
		"[v1] authReconfig",
		"[v1] linkChange",
		"[v1] initPeerAPIListener",
		"[v1] wgengine: Reconfig",
		"wgengine: Reconfig:",
		"tsdial: bart table",
		// magicsock 相关 - 保留 rebind 和 bind 失败日志用于诊断
		"magicsock: disco:",
		"magicsock: [v1]",
		"magicsock: [v2]",
		"magicsock: adding connection",
		"magicsock: active derp conns",
		"derphttp.Client",
		"LinkChange:",
		// 保留 Rebind 日志用于诊断连接问题
		// "Rebind;",
		"peerapi: serving",
		"logpolicy:",
		"ipnext: active extensions",
		"blockEngineUpdates",
		"cannot fetch existing TKA state",
		"Switching ipn state",
		"control: NetInfo:",
		"control: RegisterReq:",
		"control: LoginInteractive",
		"control: doLogin",
		"control: client.Login",
		"control: control server key",
		"control: Generating",
		"Start:",
		"Backend:",
		"StartLoginInteractive",
		// 门户检测日志（太频繁）
		"captive portal detection",
		"DetectCaptivePortal",
		// 心跳时间戳（太频繁）
		"controltime",
	}

	for _, pattern := range filterPatterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}

	return false
}
