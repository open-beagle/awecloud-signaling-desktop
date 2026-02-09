//go:build !darwin

package dns

import "log"

// ConfigureSystemDNS 配置系统 DNS（非 macOS 平台暂不实现）
// Linux/Windows 的 DNS 劫持在 P2 阶段实现
func ConfigureSystemDNS(port int) error {
	log.Printf("[DNS] 当前平台暂不支持自动 DNS 配置，请手动将 .k8s 域名指向 127.0.0.1:%d", port)
	return nil
}

// CleanupSystemDNS 清理系统 DNS 配置（非 macOS 平台暂不实现）
func CleanupSystemDNS() error {
	return nil
}
