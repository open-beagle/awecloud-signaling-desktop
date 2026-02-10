//go:build windows

package dns

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// ConfigureSystemDNS 配置 Windows 系统 DNS
// 使用 NRPT (Name Resolution Policy Table) 将 .beagle 域名指向本地 DNS 服务器
// 需要管理员权限
func ConfigureSystemDNS(port int) error {
	// 检查是否已存在 NRPT 规则
	exists, err := checkNRPTRuleExists()
	if err != nil {
		log.Printf("[DNS] 检查 NRPT 规则失败: %v", err)
	}

	if exists {
		// 先删除旧规则
		if err := removeNRPTRule(); err != nil {
			log.Printf("[DNS] 删除旧 NRPT 规则失败: %v", err)
		}
	}

	// 添加新的 NRPT 规则
	// 将 .beagle 域名指向 127.0.0.1:port
	// 注意：Windows NRPT 的 NameServers 参数格式为 "IP:端口"，需要用数组形式传递
	dnsServer := fmt.Sprintf("127.0.0.1:%d", port)
	
	// PowerShell 命令：Add-DnsClientNrptRule
	// -Namespace: 域名后缀（数组形式）
	// -NameServers: DNS 服务器地址（数组形式）
	// 使用 @() 数组语法确保参数正确传递
	psCmd := fmt.Sprintf(
		`Add-DnsClientNrptRule -Namespace @(".beagle") -NameServers @("%s")`,
		dnsServer,
	)

	cmd := exec.Command("powershell", "-Command", psCmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("添加 NRPT 规则失败（需要管理员权限）: %w\n输出: %s", err, string(output))
	}

	log.Printf("[DNS] Windows NRPT 规则已添加: .beagle → %s", dnsServer)
	return nil
}

// CleanupSystemDNS 清理 Windows 系统 DNS 配置
func CleanupSystemDNS() error {
	return removeNRPTRule()
}

// checkNRPTRuleExists 检查 NRPT 规则是否存在
func checkNRPTRuleExists() (bool, error) {
	cmd := exec.Command("powershell", "-Command", `Get-DnsClientNrptRule | Where-Object { $_.Namespace -eq ".beagle" }`)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// 如果没有规则，Get-DnsClientNrptRule 可能返回错误，这是正常的
		return false, nil
	}

	// 如果输出不为空，说明规则存在
	return len(strings.TrimSpace(string(output))) > 0, nil
}

// removeNRPTRule 删除 NRPT 规则
func removeNRPTRule() error {
	// 查找并删除所有 .beagle 的 NRPT 规则
	psCmd := `Get-DnsClientNrptRule | Where-Object { $_.Namespace -eq ".beagle" } | Remove-DnsClientNrptRule -Force`
	
	cmd := exec.Command("powershell", "-Command", psCmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// 如果没有规则可删除，这不是错误
		if strings.Contains(string(output), "Cannot find") {
			log.Printf("[DNS] 没有找到需要删除的 NRPT 规则")
			return nil
		}
		return fmt.Errorf("删除 NRPT 规则失败: %w\n输出: %s", err, string(output))
	}

	log.Printf("[DNS] Windows NRPT 规则已删除")
	return nil
}
