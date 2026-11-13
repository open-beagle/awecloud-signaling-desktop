//go:build windows

package dns

import (
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

// hiddenProcAttr 隐藏子进程窗口的 SysProcAttr
// CREATE_NO_WINDOW (0x08000000) 防止 Windows 弹出 CMD 窗口
var hiddenProcAttr = &syscall.SysProcAttr{
	HideWindow:    true,
	CreationFlags: 0x08000000,
}

var (
	nrptPromptMu       sync.Mutex
	nrptPromptedKeySet = make(map[string]bool)

	nrptRegistryPaths = []string{
		`SYSTEM\CurrentControlSet\Services\Dnscache\Parameters\DnsPolicyConfig`,
		`SOFTWARE\Policies\Microsoft\Windows NT\DnsClient\DnsPolicyConfig`,
	}
)

// RecommendedListenAddr 返回 Windows 平台推荐的 DNS 监听地址
func RecommendedListenAddr() string {
	return "127.0.0.2:53"
}

// RecommendedPort 返回 Windows 平台推荐的 DNS 监听端口
// Windows NRPT 不支持自定义端口，DNS 客户端固定向 53 端口发查询
func RecommendedPort() int {
	return 53
}

// IsSystemDNSConfigured 非提权检测当前系统生效的 NRPT 策略是否已将指定后缀指向本地监听 IP
// 优先直接读取 Windows Dnscache 注册表（<0.1ms、零子进程开销），回退使用显式字符串化 PowerShell 查询
func IsSystemDNSConfigured(suffix string, listenIP string) bool {
	ns := NormalizeManagedSuffix(suffix)
	ip := normalizeLoopbackDNSIP(listenIP)

	if isNRPTConfiguredInRegistry(ns, ip) {
		return true
	}

	psCmd := fmt.Sprintf(`$ProgressPreference = 'SilentlyContinue'
$policies = Get-DnsClientNrptPolicy -Effective -ErrorAction SilentlyContinue | Where-Object { $_.Namespace -contains '%s' -or $_.Namespace -eq '%s' }
foreach ($p in $policies) {
    foreach ($s in $p.NameServers) {
        if ($null -ne $s) { Write-Output ($s.ToString()) }
    }
}`, ns, ns)

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-EncodedCommand", encodeCommandForPowerShell(psCmd))
	cmd.SysProcAttr = hiddenProcAttr
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	return matchNRPTPowerShellOutput(string(output), ip)
}

func isNRPTConfiguredInRegistry(targetSuffix string, targetIP string) bool {
	for _, basePath := range nrptRegistryPaths {
		baseKey, err := registry.OpenKey(registry.LOCAL_MACHINE, basePath, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		subKeys, enumErr := baseKey.ReadSubKeyNames(-1)
		_ = baseKey.Close()
		if enumErr != nil {
			continue
		}

		for _, sub := range subKeys {
			ruleKey, openErr := registry.OpenKey(registry.LOCAL_MACHINE, basePath+`\`+sub, registry.QUERY_VALUE)
			if openErr != nil {
				continue
			}
			names, _, nameErr := ruleKey.GetStringsValue("Name")
			if nameErr != nil {
				if singleName, _, singleErr := ruleKey.GetStringValue("Name"); singleErr == nil && singleName != "" {
					names = []string{singleName}
				}
			}
			genericServers, _, srvErr := ruleKey.GetStringValue("GenericDNSServers")
			if srvErr != nil {
				if multiSrv, _, multiErr := ruleKey.GetStringsValue("GenericDNSServers"); multiErr == nil {
					genericServers = strings.Join(multiSrv, ";")
				}
			}
			_ = ruleKey.Close()

			if matchNRPTRuleEntry(names, genericServers, targetSuffix, targetIP) {
				return true
			}
		}
	}
	return false
}

func matchNRPTRuleEntry(names []string, genericServers string, targetSuffix string, targetIP string) bool {
	wantSuffix := NormalizeManagedSuffix(targetSuffix)
	wantIP := normalizeLoopbackDNSIP(targetIP)

	matchedNamespace := false
	for _, rawName := range names {
		for _, token := range strings.FieldsFunc(rawName, func(r rune) bool {
			return r == ';' || r == ',' || r == ' ' || r == '\t'
		}) {
			if NormalizeManagedSuffix(token) == wantSuffix {
				matchedNamespace = true
				break
			}
		}
		if matchedNamespace {
			break
		}
	}
	if !matchedNamespace {
		return false
	}

	for _, srv := range strings.FieldsFunc(genericServers, func(r rune) bool {
		return r == ';' || r == ',' || r == ' ' || r == '\t'
	}) {
		if strings.EqualFold(strings.TrimSpace(srv), wantIP) {
			return true
		}
	}
	return false
}

func matchNRPTPowerShellOutput(output string, targetIP string) bool {
	wantIP := normalizeLoopbackDNSIP(targetIP)
	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.EqualFold(line, wantIP) {
			return true
		}
		// 兼容 Windows PowerShell 5.1 将 System.Net.IPAddress 格式化为 "IPAddressToString : 127.0.0.2"
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			if strings.EqualFold(key, "IPAddressToString") && strings.EqualFold(val, wantIP) {
				return true
			}
		}
	}
	return false
}

func windowsHostsFilePath() string {
	sysRoot := strings.TrimSpace(os.Getenv("SystemRoot"))
	if sysRoot == "" {
		sysRoot = `C:\Windows`
	}
	return filepath.Join(sysRoot, "System32", "drivers", "etc", "hosts")
}

// AreManagedHostsConfigured 非提权读取系统 etc\hosts，检查是否已包含全部指定受管域名与确定性 VIP 映射
func AreManagedHostsConfigured(entries []ManagedHostEntry) bool {
	clean := SanitizeManagedHostEntries(entries)
	if len(clean) == 0 {
		return false
	}
	raw, err := os.ReadFile(windowsHostsFilePath())
	if err != nil {
		return false
	}
	return HasManagedHostsEntries(string(raw), clean)
}

// EnsureProxifierSuffixBypass 检测本机已有 Proxifier 配置文件并幂等补齐动态品牌后缀与本地 VIP 直连豁免（无需提权）
func EnsureProxifierSuffixBypass(suffix string) (bool, error) {
	if isGoTestProcess() {
		return false, nil
	}
	appData := strings.TrimSpace(os.Getenv("APPDATA"))
	if appData == "" {
		return false, nil
	}
	profilesDir := filepath.Join(appData, "Proxifier4", "Profiles")
	changed, err := EnsureProxifierSuffixBypassInDir(profilesDir, suffix)
	if changed {
		log.Printf("[DNS] 已自动更新本机 Proxifier 配置：补齐 %s 域名解析排除与 127.1.*.* 本地直连豁免", NormalizeManagedSuffix(suffix))
	}
	return changed, err
}

// ConfigureSystemDNS 兼容默认入口
func ConfigureSystemDNS(port int) error {
	return ConfigureSystemDNSForSuffix(".beagle", "127.0.0.2", port, true, false)
}

// ConfigureSystemDNSForSuffix 按需检测并持久化配置 Windows NRPT 域名后缀分流规则
func ConfigureSystemDNSForSuffix(suffix string, listenIP string, port int, allowElevation bool, forcePrompt bool) error {
	return ConfigureSystemDNSWithHosts(suffix, listenIP, port, nil, allowElevation, forcePrompt)
}

// ConfigureSystemDNSWithHosts 按需检测并持久化配置 Windows NRPT 规则与受管 etc\hosts 确定性 VIP 映射
// 当通配系统解析或受管 hosts 已就绪时静默返回（零 UAC 弹窗）；缺失时通过单次 UAC 提权原子写入 NRPT 与受管 hosts 块
func ConfigureSystemDNSWithHosts(suffix string, listenIP string, port int, entries []ManagedHostEntry, allowElevation bool, forcePrompt bool) error {
	if port != 53 {
		log.Printf("[DNS] 警告: Windows NRPT 固定查询 UDP 53 端口（当前: %d）", port)
	}

	_, _ = EnsureProxifierSuffixBypass(suffix)

	ns := NormalizeManagedSuffix(suffix)
	ip := normalizeLoopbackDNSIP(listenIP)
	cleanEntries := SanitizeManagedHostEntries(entries)

	nrptReady := IsSystemDNSConfigured(ns, ip)
	wildcardReady := nrptReady && VerifySystemDNSResolution(ns)
	hostsReady := len(cleanEntries) > 0 && AreManagedHostsConfigured(cleanEntries)

	if !forcePrompt {
		if wildcardReady {
			log.Printf("[DNS] Windows NRPT 与系统动态解析已就绪: %s -> %s:53", ns, ip)
			return nil
		}
		if nrptReady && (hostsReady || len(cleanEntries) == 0) {
			log.Printf("[DNS] Windows NRPT 策略已就绪: %s -> %s:53 (hostsReady=%v)", ns, ip, hostsReady)
			return nil
		}
	}

	if isGoTestProcess() {
		return fmt.Errorf("测试环境跳过系统 NRPT/hosts 写入 (%s -> %s)", ns, ip)
	}

	suffixes := []string{ns}
	if ns != ".beagle" {
		suffixes = append(suffixes, ".beagle")
	}
	script := buildNRPTAndHostsUpsertScript(suffixes, ip, cleanEntries)

	isFullyApplied := func() bool {
		if !IsSystemDNSConfigured(ns, ip) {
			return false
		}
		if len(cleanEntries) > 0 && !AreManagedHostsConfigured(cleanEntries) {
			return false
		}
		return true
	}

	// 1. 先尝试直接执行（若当前进程已具备管理员权限可静默成功）
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-EncodedCommand", encodeCommandForPowerShell(script))
	cmd.SysProcAttr = hiddenProcAttr
	if _, err := cmd.CombinedOutput(); err == nil && isFullyApplied() {
		log.Printf("[DNS] Windows NRPT 与受管 hosts 规则已直接写入: %s -> %s:53 (hosts=%d)", ns, ip, len(cleanEntries))
		return nil
	}

	// 2. 若允许一次性 UAC 提权，拉起 ShellExecuteExW("runas") 持久化注册
	if !allowElevation {
		return fmt.Errorf("系统域名解析规则 %s -> %s 尚未配置（需要管理员授权）", ns, ip)
	}

	promptKey := buildPromptDedupeKey(ns, ip, cleanEntries)
	nrptPromptMu.Lock()
	alreadyPrompted := nrptPromptedKeySet[promptKey]
	if alreadyPrompted && !forcePrompt {
		nrptPromptMu.Unlock()
		return fmt.Errorf("系统域名解析规则 %s -> %s 尚未启用", ns, ip)
	}
	nrptPromptedKeySet[promptKey] = true
	nrptPromptMu.Unlock()

	if err := runPowerShellElevated(script); err != nil {
		return fmt.Errorf("提权写入系统域名解析规则失败: %w", err)
	}
	if !isFullyApplied() {
		return fmt.Errorf("提权完成后未检测到生效的系统域名解析规则 (%s -> %s)", ns, ip)
	}

	log.Printf("[DNS] Windows NRPT 与受管 hosts 持久化规则已通过一次性授权生效: %s -> %s:53 (hosts=%d)", ns, ip, len(cleanEntries))
	return nil
}

func buildPromptDedupeKey(ns string, ip string, entries []ManagedHostEntry) string {
	var b strings.Builder
	b.WriteString(ns)
	b.WriteByte('@')
	b.WriteString(ip)
	for _, item := range entries {
		b.WriteByte('|')
		b.WriteString(item.Domain)
		b.WriteByte('=')
		b.WriteString(item.IP)
	}
	return b.String()
}

// CleanupSystemDNS 在 Windows 上保持空操作，使 NRPT 后缀规则与受管确定性 hosts 跨重启持久保留，彻底避免每次启动重复弹出 UAC
func CleanupSystemDNS() error {
	return nil
}

func normalizeLoopbackDNSIP(raw string) string {
	parsed := net.ParseIP(strings.TrimSpace(raw))
	if parsed != nil {
		if ip4 := parsed.To4(); ip4 != nil && ip4[0] == 127 {
			return ip4.String()
		}
	}
	return "127.0.0.2"
}

func buildNRPTUpsertScript(suffixes []string, ip string) string {
	return buildNRPTAndHostsUpsertScript(suffixes, ip, nil)
}

func buildNRPTAndHostsUpsertScript(suffixes []string, ip string, entries []ManagedHostEntry) string {
	cleanEntries := SanitizeManagedHostEntries(entries)
	var b strings.Builder
	b.WriteString("$ProgressPreference = 'SilentlyContinue'\n")
	for _, s := range suffixes {
		ns := NormalizeManagedSuffix(s)
		fmt.Fprintf(&b, "Get-DnsClientNrptRule -ErrorAction SilentlyContinue | Where-Object { $_.Namespace -contains '%s' -or $_.Namespace -eq '%s' } | Remove-DnsClientNrptRule -Force -ErrorAction SilentlyContinue\n", ns, ns)
		fmt.Fprintf(&b, "Add-DnsClientNrptRule -Namespace '%s' -NameServers '%s' -ErrorAction Stop | Out-Null\n", ns, ip)
	}
	if len(cleanEntries) > 0 {
		var block strings.Builder
		block.WriteString(ManagedHostsBeginMarker + "\r\n")
		for _, item := range cleanEntries {
			block.WriteString(item.IP + "\t" + item.Domain + "\r\n")
		}
		block.WriteString(ManagedHostsEndMarker + "\r\n")
		blockB64 := base64.StdEncoding.EncodeToString([]byte(block.String()))

		b.WriteString("$hostsPath = Join-Path $env:SystemRoot 'System32\\drivers\\etc\\hosts'\n")
		b.WriteString("$existingHosts = ''\n")
		b.WriteString("if (Test-Path -LiteralPath $hostsPath) {\n")
		b.WriteString("    $existingHosts = [System.IO.File]::ReadAllText($hostsPath, [System.Text.Encoding]::UTF8)\n")
		b.WriteString("}\n")
		b.WriteString("$hostLines = $existingHosts -replace \"`r`n\", \"`n\" -split \"`n\"\n")
		b.WriteString("$keptLines = New-Object System.Collections.Generic.List[string]\n")
		b.WriteString("$inManaged = $false\n")
		b.WriteString("foreach ($ln in $hostLines) {\n")
		b.WriteString("    $tr = $ln.Trim()\n")
		fmt.Fprintf(&b, "    if ($tr -eq '%s') { $inManaged = $true; continue }\n", ManagedHostsBeginMarker)
		fmt.Fprintf(&b, "    if ($tr -eq '%s') { $inManaged = $false; continue }\n", ManagedHostsEndMarker)
		b.WriteString("    if (-not $inManaged) { $keptLines.Add($ln) }\n")
		b.WriteString("}\n")
		b.WriteString("while ($keptLines.Count -gt 0 -and [string]::IsNullOrWhiteSpace($keptLines[$keptLines.Count - 1])) {\n")
		b.WriteString("    $keptLines.RemoveAt($keptLines.Count - 1)\n")
		b.WriteString("}\n")
		fmt.Fprintf(&b, "$managedBlock = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('%s'))\n", blockB64)
		b.WriteString("$prefixText = ''\n")
		b.WriteString("if ($keptLines.Count -gt 0) { $prefixText = ($keptLines -join \"`r`n\") + \"`r`n`r`n\" }\n")
		b.WriteString("[System.IO.File]::WriteAllText($hostsPath, ($prefixText + $managedBlock), (New-Object System.Text.UTF8Encoding($false)))\n")
	}
	b.WriteString("Clear-DnsClientCache -ErrorAction SilentlyContinue\n")
	return b.String()
}

func isGoTestProcess() bool {
	if flag.Lookup("test.v") != nil {
		return true
	}
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	lower := strings.ToLower(exe)
	return strings.HasSuffix(lower, ".test.exe") || strings.HasSuffix(lower, ".test")
}

type shellExecuteInfoW struct {
	cbSize       uint32
	fMask        uint32
	hwnd         uintptr
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     uintptr
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    uintptr
	dwHotKey     uint32
	hIcon        uintptr
	hProcess     syscall.Handle
}

func runPowerShellElevated(psScript string) error {
	const (
		seeMaskNoCloseProcess = 0x00000040
		swHide                = 0
	)

	verbPtr, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	filePtr, err := syscall.UTF16PtrFromString("powershell.exe")
	if err != nil {
		return err
	}
	params := "-NoProfile -NonInteractive -WindowStyle Hidden -EncodedCommand " + encodeCommandForPowerShell(psScript)
	paramsPtr, err := syscall.UTF16PtrFromString(params)
	if err != nil {
		return err
	}

	sei := shellExecuteInfoW{
		fMask:        seeMaskNoCloseProcess,
		lpVerb:       verbPtr,
		lpFile:       filePtr,
		lpParameters: paramsPtr,
		nShow:        swHide,
	}
	sei.cbSize = uint32(unsafe.Sizeof(sei))

	shell32 := syscall.NewLazyDLL("shell32.dll")
	procShellExecuteExW := shell32.NewProc("ShellExecuteExW")
	ret, _, callErr := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&sei)))
	if ret == 0 {
		return callErr
	}
	if sei.hProcess != 0 {
		defer syscall.CloseHandle(sei.hProcess)
		_, _ = syscall.WaitForSingleObject(sei.hProcess, 25000)
	}
	return nil
}

// encodeCommandForPowerShell 将 PowerShell 命令编码为 UTF-16LE Base64
func encodeCommandForPowerShell(cmd string) string {
	utf16Codes := utf16.Encode([]rune(cmd))
	bytes := make([]byte, len(utf16Codes)*2)
	for i, code := range utf16Codes {
		bytes[i*2] = byte(code)
		bytes[i*2+1] = byte(code >> 8)
	}
	return base64.StdEncoding.EncodeToString(bytes)
}
