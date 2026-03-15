//go:build darwin

// Package privilege 提供 macOS 平台 osascript 提权工具
package privilege

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// RunWithPrivilege 通过 osascript 以 root 权限执行 shell 命令
// macOS 会弹出系统密码输入框，用户授权后执行
func RunWithPrivilege(command string) (string, error) {
	log.Printf("[Privilege] 请求 root 权限执行: %s", command)

	// 转义命令中的双引号和反斜杠
	escaped := strings.ReplaceAll(command, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)

	script := fmt.Sprintf(`do shell script "%s" with administrator privileges`, escaped)
	cmd := exec.Command("osascript", "-e", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("osascript 执行失败: %w, output: %s", err, string(output))
	}
	return strings.TrimSpace(string(output)), nil
}

// RunBatch 批量执行需要 root 权限的命令（合并为一次授权弹窗）
func RunBatch(commands []string) error {
	if len(commands) == 0 {
		return nil
	}
	combined := strings.Join(commands, " && ")
	_, err := RunWithPrivilege(combined)
	return err
}
