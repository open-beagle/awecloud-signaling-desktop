//go:build windows

// 平台特定实现 - Windows
package tailscale

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/tailscale/wireguard-go/tun"
	"golang.org/x/sys/windows"
)

// 固定 GUID，用于复用设备
var beagleTunGUID = windows.GUID{
	Data1: 0x8C5E2B3A,
	Data2: 0xF7D1,
	Data3: 0x4E9A,
	Data4: [8]byte{0xB6, 0xC8, 0x1A, 0x2D, 0x3E, 0x4F, 0x56, 0x78},
}

func init() {
	// 设置 Wintun 参数
	// TunnelType (pool): 必须与之前创建设备时一致，否则无法复用
	tun.WintunTunnelType = "Beagle"
	tun.WintunStaticRequestedGUID = &beagleTunGUID
}

// PlatformInit 平台初始化
// Windows: 预加载 wintun.dll
func PlatformInit() error {
	return preloadWintun()
}

// GetTunName 获取 TUN 设备名称
func GetTunName() string {
	return "btun"
}

// CreateTUN 创建或复用 TUN 设备
// 使用固定 GUID，wintun 会自动复用同 GUID 的设备
// 参考 tailscale 的 tstunNewWithWindowsRetries，使用重试机制
func CreateTUN() (tun.Device, string, error) {
	tunName := GetTunName()
	mtu := 1280 // 默认 MTU

	var lastErr error
	// 重试最多 30 次，每次间隔 1 秒，总共 30 秒
	for i := 0; i < 30; i++ {
		if i > 0 {
			log.Printf("[INFO] [Tunnel] 重试创建 TUN 设备 (%d/30)...", i+1)
			time.Sleep(1 * time.Second)
		}

		dev, err := tun.CreateTUNWithRequestedGUID(tunName, &beagleTunGUID, mtu)
		if err == nil {
			log.Printf("[INFO] [Tunnel] TUN 设备已创建/复用: %s", tunName)
			return dev, tunName, nil
		}
		lastErr = err

		// 如果是 "file already exists" 错误，尝试清理
		if i == 0 {
			log.Printf("[WARN] [Tunnel] 创建失败: %v", err)
		}
	}

	return nil, "", fmt.Errorf("创建 TUN 设备失败（重试 30 次）: %w", lastErr)
}

// preloadWintun 预加载 wintun.dll
// 必须使用完整路径加载，避免加载 system32 中的旧版本
func preloadWintun() error {
	// 优先从可执行文件所在目录加载
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取可执行文件路径失败: %w", err)
	}
	exeDir := filepath.Dir(exe)

	// 获取当前工作目录
	cwd, _ := os.Getwd()

	// 尝试多个可能的路径（按优先级排序）
	searchPaths := []string{
		filepath.Join(exeDir, "wintun.dll"),              // 可执行文件目录（最优先，build/bin/）
		filepath.Join(cwd, "build", "bin", "wintun.dll"), // 工作目录下的 build/bin（dev 模式）
		filepath.Join(cwd, "wintun.dll"),                 // 工作目录
		filepath.Join(".", "wintun.dll"),                 // 当前目录
	}

	log.Printf("[DEBUG] [Tunnel] 搜索 DLL, exe=%s, cwd=%s", exeDir, cwd)

	for _, dllPath := range searchPaths {
		absPath, err := filepath.Abs(dllPath)
		if err != nil {
			continue
		}

		if _, err := os.Stat(absPath); err != nil {
			continue
		}

		log.Printf("[DEBUG] [Tunnel] 加载 DLL: %s", absPath)

		// 使用 windows.LoadDLL 而非 syscall.LoadDLL
		// windows.LoadDLL 是推荐的方式，与 tailscale 源码一致
		if _, err := windows.LoadDLL(absPath); err != nil {
			log.Printf("[WARN] [Tunnel] DLL 加载失败: %v", err)
			continue
		}

		return nil
	}

	return fmt.Errorf("wintun.dll 未找到，搜索路径: exe=%s, cwd=%s", exeDir, cwd)
}
