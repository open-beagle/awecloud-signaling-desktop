//go:build darwin

// 平台特定实现 - macOS
package tailscale

// PlatformInit 平台初始化
// macOS: 使用系统原生 utun，无需额外初始化
func PlatformInit() error {
	// macOS 使用系统原生的 utun 接口
	// 由 wireguard-go 自动处理，无需预加载驱动
	return nil
}

// GetTunName 获取 TUN 设备名称
// macOS 的 utun 名称会被系统忽略，自动分配编号（如 utun0, utun1）
func GetTunName() string {
	return "btun"
}
