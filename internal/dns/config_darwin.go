//go:build darwin

package dns

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/privilege"
)

const resolverDir = "/etc/resolver"
const resolverFile = "beagle"                        // 域名后缀
const resolverHeader = "# Added by Signal Desktop\n" // 文件标记，用于清理时识别

// RecommendedListenAddr 返回 macOS 平台推荐的 DNS 监听地址
// macOS 默认只有 127.0.0.1 可用（127.0.0.2 需要手动添加 loopback alias）
// 使用 127.0.0.1:15353 高端口，通过 /etc/resolver 的 port 指令转发
func RecommendedListenAddr() string {
	return "127.0.0.1:15353"
}

// RecommendedPort 返回 macOS 平台推荐的 DNS 监听端口
func RecommendedPort() int {
	return 15353
}

// IsSystemDNSConfigured 检查 macOS /etc/resolver/<suffix> 是否已指向本地端口
func IsSystemDNSConfigured(suffix string, _ string) bool {
	label := strings.TrimPrefix(NormalizeManagedSuffix(suffix), ".")
	data, err := os.ReadFile(filepath.Join(resolverDir, label))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "nameserver 127.0.0.1")
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

// ConfigureSystemDNSWithHosts 配置系统 DNS（macOS 通过 /etc/resolver/<label> 原生支持按后缀端口分流）
func ConfigureSystemDNSWithHosts(suffix string, listenIP string, port int, _ []ManagedHostEntry, allowElevation bool, forcePrompt bool) error {
	return ConfigureSystemDNSForSuffix(suffix, listenIP, port, allowElevation, forcePrompt)
}

// ConfigureSystemDNS 配置系统 DNS，将 .beagle 域名指向本地 DNS 服务器
func ConfigureSystemDNS(port int) error {
	return ConfigureSystemDNSForSuffix(".beagle", "127.0.0.1", port, true, false)
}

// ConfigureSystemDNSForSuffix 按动态品牌后缀写入 macOS /etc/resolver/<label>
func ConfigureSystemDNSForSuffix(suffix string, _ string, port int, allowElevation bool, _ bool) error {
	label := strings.TrimPrefix(NormalizeManagedSuffix(suffix), ".")
	content := fmt.Sprintf("%snameserver 127.0.0.1\nport %d\n", resolverHeader, port)
	resolverPath := filepath.Join(resolverDir, label)

	if existing, err := os.ReadFile(resolverPath); err == nil && string(existing) == content {
		return nil
	}
	if !allowElevation {
		return fmt.Errorf("resolver %s 尚未配置", resolverPath)
	}

	// 通过 osascript 提权创建目录和写入文件
	cmd := fmt.Sprintf("mkdir -p %s && printf '%%s' '%s' > %s",
		resolverDir, content, resolverPath)
	if _, err := privilege.RunWithPrivilege(cmd); err != nil {
		return fmt.Errorf("创建 %s 失败（需要管理员权限）: %w", resolverPath, err)
	}

	log.Printf("[DNS] macOS DNS 配置已写入: %s (port=%d)", resolverPath, port)
	return nil
}

// CleanupSystemDNS 清理系统 DNS 配置
// 只删除带有 Signal Desktop 标记的文件
func CleanupSystemDNS() error {
	resolverPath := filepath.Join(resolverDir, resolverFile)

	// 先检查文件是否存在
	data, err := os.ReadFile(resolverPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		// 可能没有读取权限，尝试直接删除
	}

	// 只删除带有我们标记的文件
	if len(data) > 0 && string(data[:min(len(data), len(resolverHeader))]) != resolverHeader {
		log.Printf("[DNS] %s 不是由 Signal Desktop 创建的，跳过删除", resolverPath)
		return nil
	}

	// 通过 osascript 提权删除文件
	if _, err := privilege.RunWithPrivilege(fmt.Sprintf("rm -f %s", resolverPath)); err != nil {
		return fmt.Errorf("删除 %s 失败: %w", resolverPath, err)
	}

	log.Printf("[DNS] macOS DNS 配置已清理")
	return nil
}
