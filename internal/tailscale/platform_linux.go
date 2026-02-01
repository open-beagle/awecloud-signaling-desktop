//go:build linux

// 平台特定实现 - Linux
package tailscale

// PlatformInit 平台初始化
// Linux: 使用 /dev/net/tun，无需额外初始化
func PlatformInit() error {
	// Linux 使用标准的 /dev/net/tun 接口
	// 需要 root 权限或 CAP_NET_ADMIN capability
	return nil
}

// GetTunName 获取 TUN 设备名称
func GetTunName() string {
	return "btun"
}
