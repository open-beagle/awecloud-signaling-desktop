//go:build !darwin && !windows && !linux

package dns

import "log"

// RecommendedListenAddr 返回默认平台推荐的 DNS 监听地址
func RecommendedListenAddr() string {
	return "127.0.0.2:53"
}

// RecommendedPort 返回 Linux 平台推荐的 DNS 监听端口
func RecommendedPort() int {
	return 53
}

// IsSystemDNSConfigured 默认平台返回 false
func IsSystemDNSConfigured(_ string, _ string) bool {
	return false
}

// ConfigureSystemDNS 配置系统 DNS
func ConfigureSystemDNS(port int) error {
	return ConfigureSystemDNSForSuffix(".beagle", "127.0.0.2", port, false, false)
}

// ConfigureSystemDNSForSuffix 默认平台提示手动配置
func ConfigureSystemDNSForSuffix(suffix string, listenIP string, port int, _ bool, _ bool) error {
	log.Printf("[DNS] 当前平台暂不支持自动 DNS 配置，请手动将 %s 域名指向 %s:%d", suffix, listenIP, port)
	return nil
}

// CleanupSystemDNS 清理系统 DNS 配置（Linux 平台暂不实现）
func CleanupSystemDNS() error {
	return nil
}
