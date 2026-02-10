//go:build !darwin && !windows

package dns

import "log"

// RecommendedPort 返回 Linux 平台推荐的 DNS 监听端口
func RecommendedPort() int {
	return 53
}

// ConfigureSystemDNS 配置系统 DNS（Linux 平台暂不实现）
// Linux 的 DNS 劫持在 P2 阶段实现（systemd-resolved 或 /etc/resolv.conf）
func ConfigureSystemDNS(port int) error {
	log.Printf("[DNS] Linux 平台暂不支持自动 DNS 配置，请手动将 .beagle 域名指向 127.0.0.2:%d", port)
	log.Printf("[DNS] 或者使用 IP 地址直接连接 Agent")
	return nil
}

// CleanupSystemDNS 清理系统 DNS 配置（Linux 平台暂不实现）
func CleanupSystemDNS() error {
	return nil
}
