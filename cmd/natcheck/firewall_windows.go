//go:build windows

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unicode/utf16"
)

const stagingFirewallRuleName = "SignalDesktop-Staging-UDP-In"

type psFirewallProbeOutput struct {
	IsElevated        bool     `json:"is_elevated"`
	ActiveProfiles    []string `json:"active_profiles"`
	HasBlockUDP       bool     `json:"has_block_udp"`
	AllowRuleProfiles []string `json:"allow_rule_profiles"`
}

func resolveTargetDesktopBinaryPath() (string, bool) {
	candidates := []string{
		filepath.Join("build", "bin", "awecloud-signaling-desktop.exe"),
		filepath.Join("desktop", "build", "bin", "awecloud-signaling-desktop.exe"),
		filepath.Join("..", "desktop", "build", "bin", "awecloud-signaling-desktop.exe"),
	}
	for _, c := range candidates {
		if abs, err := filepath.Abs(c); err == nil {
			if st, statErr := os.Stat(abs); statErr == nil && !st.IsDir() {
				return abs, true
			}
		}
	}
	if abs, err := filepath.Abs(filepath.Join("build", "bin", "awecloud-signaling-desktop.exe")); err == nil {
		return abs, false
	}
	return "", false
}

func inspectFirewallStatus() FirewallStatus {
	targetExe, exists := resolveTargetDesktopBinaryPath()
	status := FirewallStatus{
		OS:                 runtime.GOOS,
		TargetBinaryExists: exists,
		Verdict:            "unknown",
	}

	escapedExe := strings.ReplaceAll(targetExe, "'", "''")
	psScript := fmt.Sprintf(`
$ErrorActionPreference = 'SilentlyContinue'
$ProgressPreference = 'SilentlyContinue'
$WarningPreference = 'SilentlyContinue'
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
$profiles = @(Get-NetConnectionProfile | Where-Object { $_.IPv4Connectivity -ne 'Disconnected' -or $_.IPv6Connectivity -ne 'Disconnected' } | ForEach-Object { [string]$_.NetworkCategory })
if ($profiles.Count -eq 0) {
    $profiles = @(Get-NetConnectionProfile | ForEach-Object { [string]$_.NetworkCategory })
}
$target = '%s'
$hasBlock = $false
$allowProfiles = @()
$candidateRules = @()
if ($target) {
    $afs = @(Get-NetFirewallApplicationFilter -Program $target -ErrorAction SilentlyContinue)
    foreach ($af in $afs) {
        $r = Get-NetFirewallRule -AssociatedNetFirewallApplicationFilter $af -ErrorAction SilentlyContinue
        if ($r) { $candidateRules += $r }
    }
}
$namedRules = @(Get-NetFirewallRule -DisplayName 'SignalDesktop*' -ErrorAction SilentlyContinue)
if ($namedRules.Count -gt 0) {
    $candidateRules += $namedRules
}
foreach ($rule in ($candidateRules | Sort-Object -Property Name -Unique)) {
    if (-not $rule -or ([string]$rule.Enabled) -ne 'True' -or ([string]$rule.Direction) -ne 'Inbound') { continue }
    $pf = Get-NetFirewallPortFilter -AssociatedNetFirewallRule $rule -ErrorAction SilentlyContinue
    $proto = [string]$pf.Protocol
    if ($proto -ne 'UDP' -and $proto -ne 'Any') { continue }
    $act = [string]$rule.Action
    if ($act -eq 'Block') { $hasBlock = $true }
    if ($act -eq 'Allow') { $allowProfiles += [string]$rule.Profile }
}
[pscustomobject]@{
    is_elevated = [bool]$isAdmin
    active_profiles = @($profiles | Select-Object -Unique)
    has_block_udp = [bool]$hasBlock
    allow_rule_profiles = @($allowProfiles | Select-Object -Unique)
} | ConvertTo-Json -Compress
`, escapedExe)

	out, err := runHiddenPowerShell(psScript)
	if err != nil {
		status.Summary = "无法读取 Windows Defender 防火墙状态"
		return status
	}

	var parsed psFirewallProbeOutput
	if jErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &parsed); jErr != nil {
		status.Summary = "解析 Windows Defender 防火墙状态失败"
		return status
	}

	status.IsElevated = parsed.IsElevated
	status.ActiveProfiles = parsed.ActiveProfiles
	status.HasExplicitBlockUDP = parsed.HasBlockUDP
	status.AllowRuleProfiles = parsed.AllowRuleProfiles
	status.HasAllowUDPForProfile = evaluateProfileCoverage(parsed.ActiveProfiles, parsed.AllowRuleProfiles)

	switch {
	case parsed.HasBlockUDP:
		status.Verdict = "blocked"
		status.Summary = "存在显式阻止测试版 Desktop 入站 UDP 的防火墙规则"
	case status.HasAllowUDPForProfile:
		status.Verdict = "allow"
		status.Summary = "测试版 Desktop 已具备覆盖当前网络类型的 UDP 入站放行规则"
	default:
		isPublic := false
		for _, p := range parsed.ActiveProfiles {
			if strings.Contains(strings.ToLower(p), "public") {
				isPublic = true
				break
			}
		}
		if isPublic || len(parsed.AllowRuleProfiles) == 0 {
			status.Verdict = "missing_rule_public"
			status.Summary = "缺少针对测试版 Desktop 的 UDP 入站放行规则，Windows 默认丢弃未请求的对端打洞包"
		} else {
			status.Verdict = "allow"
			status.Summary = "当前处于专用/域网络且无显式阻止规则"
		}
	}
	return status
}

func evaluateProfileCoverage(activeProfiles, allowProfiles []string) bool {
	if len(allowProfiles) == 0 {
		return false
	}
	joinedAllow := strings.ToLower(strings.Join(allowProfiles, ","))
	if strings.Contains(joinedAllow, "any") {
		return true
	}
	if len(activeProfiles) == 0 {
		return len(allowProfiles) > 0
	}
	for _, act := range activeProfiles {
		actLower := strings.ToLower(strings.TrimSpace(act))
		if actLower == "" {
			continue
		}
		if strings.Contains(actLower, "domain") {
			actLower = "domain"
		}
		if !strings.Contains(joinedAllow, actLower) {
			return false
		}
	}
	return true
}

func applyStagingUDPFirewallFix() error {
	targetExe, _ := resolveTargetDesktopBinaryPath()
	if targetExe == "" {
		return fmt.Errorf("未定位到 desktop\\build\\bin\\awecloud-signaling-desktop.exe 路径")
	}
	escapedExe := strings.ReplaceAll(targetExe, "'", "''")
	psScript := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    throw '需要管理员权限，请在管理员 PowerShell 中运行 --fix-firewall'
}
$target = '%s'
$appFilters = @(Get-NetFirewallApplicationFilter -Program $target -ErrorAction SilentlyContinue)
foreach ($af in $appFilters) {
    $rule = Get-NetFirewallRule -AssociatedNetFirewallApplicationFilter $af -ErrorAction SilentlyContinue
    if ($rule -and ([string]$rule.Direction) -eq 'Inbound' -and ([string]$rule.Action) -eq 'Block') {
        $pf = Get-NetFirewallPortFilter -AssociatedNetFirewallRule $rule -ErrorAction SilentlyContinue
        if (([string]$pf.Protocol) -eq 'UDP' -or ([string]$pf.Protocol) -eq 'Any') {
            Remove-NetFirewallRule -Name $rule.Name -ErrorAction SilentlyContinue
        }
    }
}
$existing = Get-NetFirewallRule -DisplayName '%s' -ErrorAction SilentlyContinue
if ($existing) {
    Remove-NetFirewallRule -DisplayName '%s' -ErrorAction SilentlyContinue
}
New-NetFirewallRule -DisplayName '%s' -Direction Inbound -Action Allow -Protocol UDP -Program $target -Profile Any | Out-Null
Write-Output 'OK'
`, escapedExe, stagingFirewallRuleName, stagingFirewallRuleName, stagingFirewallRuleName)

	out, err := runHiddenPowerShell(psScript)
	if err != nil {
		return fmt.Errorf("%v (%s)", err, strings.TrimSpace(out))
	}
	return nil
}

func runHiddenPowerShell(script string) (string, error) {
	u16 := utf16.Encode([]rune(script))
	raw := make([]byte, len(u16)*2)
	for i, c := range u16 {
		raw[i*2] = byte(c)
		raw[i*2+1] = byte(c >> 8)
	}
	enc := base64.StdEncoding.EncodeToString(raw)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-EncodedCommand", enc)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}
	out, err := cmd.Output()
	return string(out), err
}
