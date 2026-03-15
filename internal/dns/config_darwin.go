//go:build darwin

package dns

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

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

// ConfigureSystemDNS 配置系统 DNS，将 .beagle 域名指向本地 DNS 服务器
// macOS: 通过 osascript 提权创建 /etc/resolver/beagle 文件
func ConfigureSystemDNS(port int) error {
	content := fmt.Sprintf("%snameserver 127.0.0.1\nport %d\n", resolverHeader, port)
	resolverPath := filepath.Join(resolverDir, resolverFile)

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
