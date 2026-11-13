//go:build linux

package dns

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

// RecommendedListenAddr 返回 Linux 平台推荐的 DNS 监听地址
func RecommendedListenAddr() string {
	return "127.0.0.2:5353"
}

// RecommendedPort 返回 Linux 平台推荐的 DNS 监听端口
// systemd-resolved 占用 53 端口，Desktop DNS 使用 5353 避免冲突
func RecommendedPort() int {
	return 5353
}

// IsSystemDNSConfigured 检查 Linux systemd-resolved 是否已包含指定后缀分流配置
func IsSystemDNSConfigured(suffix string, listenIP string) bool {
	label := strings.TrimPrefix(NormalizeManagedSuffix(suffix), ".")
	data, err := os.ReadFile("/etc/systemd/resolved.conf.d/" + label + ".conf")
	if err == nil && strings.Contains(string(data), "~"+label) {
		return true
	}
	return false
}

// AreManagedHostsConfigured 检查系统 /etc/hosts 是否已包含指定受管域名映射
func AreManagedHostsConfigured(entries []ManagedHostEntry) bool {
	clean := SanitizeManagedHostEntries(entries)
	if len(clean) == 0 {
		return false
	}
	raw, err := os.ReadFile("/etc/hosts")
	if err != nil {
		return false
	}
	return HasManagedHostsEntries(string(raw), clean)
}

// EnsureProxifierSuffixBypass 非 Windows 平台无需处理 Windows Proxifier 配置
func EnsureProxifierSuffixBypass(_ string) (bool, error) {
	return false, nil
}

// ConfigureSystemDNSWithHosts 配置 Linux 系统 DNS
func ConfigureSystemDNSWithHosts(suffix string, listenIP string, port int, _ []ManagedHostEntry, allowElevation bool, forcePrompt bool) error {
	return ConfigureSystemDNSForSuffix(suffix, listenIP, port, allowElevation, forcePrompt)
}

// ConfigureSystemDNS 配置 Linux 系统 DNS，将 .beagle 域名指向本地 DNS 服务器
func ConfigureSystemDNS(port int) error {
	return ConfigureSystemDNSForSuffix(".beagle", "127.0.0.2", port, true, false)
}

// ConfigureSystemDNSForSuffix 配置 Linux 系统 DNS，将动态品牌后缀指向本地 DNS 服务器
func ConfigureSystemDNSForSuffix(suffix string, listenIP string, port int, _ bool, _ bool) error {
	label := strings.TrimPrefix(NormalizeManagedSuffix(suffix), ".")
	if listenIP == "" {
		listenIP = "127.0.0.2"
	}
	if isSystemdResolvedRunning() {
		confDir := "/etc/systemd/resolved.conf.d"
		confFile := confDir + "/" + label + ".conf"
		content := fmt.Sprintf("[Resolve]\nDNS=%s:%d\nDomains=~%s ~beagle\n", listenIP, port, label)
		if err := os.MkdirAll(confDir, 0755); err == nil {
			if err := os.WriteFile(confFile, []byte(content), 0644); err == nil {
				_ = exec.Command("systemctl", "restart", "systemd-resolved").Run()
				return nil
			}
		}
		return configureResolvectl(port)
	}
	log.Printf("[DNS] systemd-resolved 未运行，请手动将 .%s 域名指向 %s:%d", label, listenIP, port)
	return nil
}

// CleanupSystemDNS 清理 Linux 系统 DNS 配置
func CleanupSystemDNS() error {
	if isSystemdResolvedRunning() {
		return cleanupSystemdResolved()
	}
	return nil
}

// isSystemdResolvedRunning 检查 systemd-resolved 是否运行
func isSystemdResolvedRunning() bool {
	cmd := exec.Command("systemctl", "is-active", "--quiet", "systemd-resolved")
	return cmd.Run() == nil
}

// configureSystemdResolved 通过 systemd-resolved 配置 DNS 转发
// 创建 /etc/systemd/resolved.conf.d/beagle.conf 配置文件
func configureSystemdResolved(port int) error {
	confDir := "/etc/systemd/resolved.conf.d"
	confFile := confDir + "/beagle.conf"

	// 创建配置目录
	if err := os.MkdirAll(confDir, 0755); err != nil {
		// 权限不足时尝试 resolvectl 方式
		log.Printf("[DNS] 创建 %s 失败: %v，尝试 resolvectl 方式", confDir, err)
		return configureResolvectl(port)
	}

	// 写入配置文件：将 ~beagle 域名路由到本地 DNS
	// [Resolve] 段的 DNS 和 Domains 配置
	content := fmt.Sprintf("[Resolve]\nDNS=127.0.0.2:%d\nDomains=~beagle\n", port)
	if err := os.WriteFile(confFile, []byte(content), 0644); err != nil {
		log.Printf("[DNS] 写入 %s 失败: %v，尝试 resolvectl 方式", confFile, err)
		return configureResolvectl(port)
	}

	// 重启 systemd-resolved 使配置生效
	cmd := exec.Command("systemctl", "restart", "systemd-resolved")
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Printf("[DNS] 重启 systemd-resolved 失败: %v, 输出: %s", err, string(output))
		// 不返回错误，配置文件已写入，下次重启会生效
	}

	log.Printf("[DNS] Linux DNS 配置已写入: %s (.beagle → 127.0.0.2:%d)", confFile, port)
	return nil
}

// configureResolvectl 通过 resolvectl 命令配置 DNS（无需 root 写文件权限）
func configureResolvectl(port int) error {
	// 查找默认网络接口
	iface := getDefaultInterface()
	if iface == "" {
		log.Printf("[DNS] 未找到默认网络接口，请手动配置 DNS")
		return fmt.Errorf("未找到默认网络接口")
	}

	// 设置 DNS 路由域名
	cmd := exec.Command("resolvectl", "domain", iface, "~beagle")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("resolvectl domain 失败: %w, 输出: %s", err, string(output))
	}

	// 设置 DNS 服务器
	dnsAddr := fmt.Sprintf("127.0.0.2:%d", port)
	cmd = exec.Command("resolvectl", "dns", iface, dnsAddr)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("resolvectl dns 失败: %w, 输出: %s", err, string(output))
	}

	log.Printf("[DNS] Linux DNS 已通过 resolvectl 配置: %s (.beagle → %s)", iface, dnsAddr)
	return nil
}

// cleanupSystemdResolved 清理 systemd-resolved 配置
func cleanupSystemdResolved() error {
	confFile := "/etc/systemd/resolved.conf.d/beagle.conf"

	if err := os.Remove(confFile); err != nil && !os.IsNotExist(err) {
		log.Printf("[DNS] 删除 %s 失败: %v", confFile, err)
		// 不返回错误，尝试继续清理
	}

	// 重启 systemd-resolved
	cmd := exec.Command("systemctl", "restart", "systemd-resolved")
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Printf("[DNS] 重启 systemd-resolved 失败: %v, 输出: %s", err, string(output))
	}

	log.Printf("[DNS] Linux DNS 配置已清理")
	return nil
}

// getDefaultInterface 获取默认网络接口名称
func getDefaultInterface() string {
	// 通过 ip route 获取默认路由的接口
	cmd := exec.Command("ip", "route", "show", "default")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	// 解析 "default via x.x.x.x dev eth0 ..." 格式
	fields := strings.Fields(string(output))
	for i, f := range fields {
		if f == "dev" && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return ""
}
