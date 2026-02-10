//go:build darwin

package dns

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

const resolverDir = "/etc/resolver"
const resolverFile = "beagle" // 域名后缀

// RecommendedPort 返回 macOS 平台推荐的 DNS 监听端口
func RecommendedPort() int {
	return 53
}

// ConfigureSystemDNS 配置系统 DNS，将 .beagle 域名指向本地 DNS 服务器
// macOS: 创建 /etc/resolver/beagle 文件
func ConfigureSystemDNS(port int) error {
	content := fmt.Sprintf("nameserver 127.0.0.2\nport %d\n", port)

	// 创建 /etc/resolver 目录（需要 sudo 权限）
	if err := os.MkdirAll(resolverDir, 0755); err != nil {
		return fmt.Errorf("创建 %s 目录失败（可能需要 sudo 权限）: %w", resolverDir, err)
	}

	resolverPath := filepath.Join(resolverDir, resolverFile)
	if err := os.WriteFile(resolverPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("写入 %s 失败（可能需要 sudo 权限）: %w", resolverPath, err)
	}

	log.Printf("[DNS] macOS DNS 配置已写入: %s", resolverPath)
	return nil
}

// CleanupSystemDNS 清理系统 DNS 配置
func CleanupSystemDNS() error {
	resolverPath := filepath.Join(resolverDir, resolverFile)
	if err := os.Remove(resolverPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除 %s 失败: %w", resolverPath, err)
	}
	log.Printf("[DNS] macOS DNS 配置已清理")
	return nil
}
