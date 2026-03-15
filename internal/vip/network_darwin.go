//go:build darwin

// macOS VIP 网络配置：通过 ifconfig lo0 alias 添加 loopback 别名
// macOS 默认只有 127.0.0.1，127.1.x.x 需要逐个添加
// alias 不持久化，重启后自动消失，异常退出残留无害
package vip

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/privilege"
)

// NetworkConfig macOS 网络配置管理器
type NetworkConfig struct {
	aliases []string   // 已添加的 loopback alias 列表
	mu      sync.Mutex // 保护 aliases 列表
}

// NewNetworkConfig 创建网络配置管理器
func NewNetworkConfig() *NetworkConfig {
	return &NetworkConfig{
		aliases: make([]string, 0),
	}
}

// Setup macOS 上初始化时不需要预配置（alias 按需添加）
func (n *NetworkConfig) Setup() error {
	log.Printf("[Network] macOS 网络配置管理器已初始化（VIP alias 按需添加）")
	return nil
}

// Cleanup 清理所有已添加的 loopback alias
func (n *NetworkConfig) Cleanup() {
	n.mu.Lock()
	defer n.mu.Unlock()

	if len(n.aliases) == 0 {
		return
	}

	// 批量构建删除命令
	cmds := make([]string, 0, len(n.aliases))
	for _, vip := range n.aliases {
		cmds = append(cmds, fmt.Sprintf("ifconfig lo0 -alias %s", vip))
	}

	log.Printf("[Network] 清理 %d 个 loopback alias", len(n.aliases))
	if err := privilege.RunBatch(cmds); err != nil {
		// 清理失败不是致命错误，macOS 重启后 alias 会自动消失
		log.Printf("[Network] 清理 loopback alias 失败: %v（重启后自动消失）", err)
		return
	}

	log.Printf("[Network] 已清理 %d 个 loopback alias", len(n.aliases))
	n.aliases = n.aliases[:0]
}

// AddAlias 为指定 VIP 添加 loopback alias
func (n *NetworkConfig) AddAlias(vip string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// 检查是否已添加
	for _, existing := range n.aliases {
		if existing == vip {
			return nil
		}
	}

	// 检查系统上是否已存在（可能是上次异常退出残留）
	if isAliasExists(vip) {
		log.Printf("[Network] loopback alias %s 已存在，跳过添加", vip)
		n.aliases = append(n.aliases, vip)
		return nil
	}

	// 通过 osascript 提权添加 alias
	if _, err := privilege.RunWithPrivilege(fmt.Sprintf("ifconfig lo0 alias %s", vip)); err != nil {
		return fmt.Errorf("添加 loopback alias %s 失败: %w", vip, err)
	}

	n.aliases = append(n.aliases, vip)
	log.Printf("[Network] 已添加 loopback alias: %s", vip)
	return nil
}

// isAliasExists 检查 loopback alias 是否已存在
func isAliasExists(vip string) bool {
	cmd := exec.Command("ifconfig", "lo0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), vip)
}
