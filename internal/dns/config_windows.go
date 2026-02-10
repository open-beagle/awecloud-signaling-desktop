//go:build windows

package dns

import (
	"encoding/base64"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"unicode/utf16"
)

// RecommendedPort 返回 Windows 平台推荐的 DNS 监听端口
// Windows NRPT 不支持自定义端口，DNS 客户端固定向 53 端口发查询
func RecommendedPort() int {
	return 53
}

// ConfigureSystemDNS 配置 Windows 系统 DNS
// 使用 NRPT (Name Resolution Policy Table) 将 .beagle 域名指向本地 DNS 服务器
// 注意：NRPT 的 NameServers 不支持自定义端口，Windows DNS 客户端固定向 53 端口发查询
// 因此 Windows 上 DNS 服务器必须监听 127.0.0.1:53
// 需要管理员权限
func ConfigureSystemDNS(port int) error {
	if port != 53 {
		log.Printf("[DNS] 警告: Windows NRPT 不支持自定义端口，DNS 服务器需监听 53 端口（当前: %d）", port)
		log.Printf("[DNS] 请确保 DNS 服务器监听在 127.0.0.2:53，或手动配置 DNS")
	}

	// 检查并删除已存在的 NRPT 规则
	exists, err := checkNRPTRuleExists()
	if err != nil {
		log.Printf("[DNS] 检查 NRPT 规则失败: %v", err)
	}

	if exists {
		if err := removeNRPTRule(); err != nil {
			log.Printf("[DNS] 删除旧 NRPT 规则失败: %v", err)
		}
	}

	// 添加新的 NRPT 规则
	// 使用 -NameServers 参数（非 DirectAccess 模式），不需要布尔参数，避免 $true 转义问题
	psCmd := `Add-DnsClientNrptRule -Namespace ".beagle" -NameServers "127.0.0.2"`

	// 使用 Base64 编码执行，彻底避免参数解析问题
	encodedCmd := encodeCommandForPowerShell(psCmd)

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-EncodedCommand", encodedCmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("添加 NRPT 规则失败（需要管理员权限）: %w\n输出: %s", err, string(output))
	}

	log.Printf("[DNS] Windows NRPT 规则已添加: .beagle → 127.0.0.2:53")
	return nil
}

// CleanupSystemDNS 清理 Windows 系统 DNS 配置
func CleanupSystemDNS() error {
	return removeNRPTRule()
}

// checkNRPTRuleExists 检查 NRPT 规则是否存在
func checkNRPTRuleExists() (bool, error) {
	// 使用 Base64 编码避免 $_ 等特殊字符的转义问题
	psCmd := `Get-DnsClientNrptRule | Where-Object { $_.Namespace -eq ".beagle" }`
	encodedCmd := encodeCommandForPowerShell(psCmd)

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-EncodedCommand", encodedCmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// 没有规则时可能返回错误，这是正常的
		return false, nil
	}

	// 输出不为空说明规则存在
	return len(strings.TrimSpace(string(output))) > 0, nil
}

// removeNRPTRule 删除 NRPT 规则
func removeNRPTRule() error {
	// 使用 Base64 编码避免 $_ 等特殊字符的转义问题
	psCmd := `Get-DnsClientNrptRule | Where-Object { $_.Namespace -eq ".beagle" } | Remove-DnsClientNrptRule -Force`
	encodedCmd := encodeCommandForPowerShell(psCmd)

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-EncodedCommand", encodedCmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// 没有规则可删除不算错误
		if strings.Contains(string(output), "Cannot find") {
			log.Printf("[DNS] 没有找到需要删除的 NRPT 规则")
			return nil
		}
		return fmt.Errorf("删除 NRPT 规则失败: %w\n输出: %s", err, string(output))
	}

	log.Printf("[DNS] Windows NRPT 规则已删除")
	return nil
}

// encodeCommandForPowerShell 将 PowerShell 命令编码为 UTF-16LE Base64
// PowerShell -EncodedCommand 参数要求命令使用 UTF-16LE 编码并转换为 Base64
func encodeCommandForPowerShell(command string) string {
	utf16Bytes := encodeUTF16LE(command)
	return base64.StdEncoding.EncodeToString(utf16Bytes)
}

// encodeUTF16LE 将字符串编码为 UTF-16LE 字节数组
func encodeUTF16LE(s string) []byte {
	utf16Codes := utf16.Encode([]rune(s))
	bytes := make([]byte, len(utf16Codes)*2)
	for i, code := range utf16Codes {
		bytes[i*2] = byte(code)
		bytes[i*2+1] = byte(code >> 8)
	}
	return bytes
}
