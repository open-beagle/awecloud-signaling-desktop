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

// ConfigureSystemDNS 兼容默认入口
func ConfigureSystemDNS(port int) error {
	return ConfigureSystemDNSForSuffix(".beagle", "127.0.0.2", port, true, false)
}

// ConfigureSystemDNSForSuffix 按需检测并持久化配置 Windows NRPT 域名后缀分流规则
// 已存在匹配规则时直接静默返回（零 UAC 弹窗）；缺失时支持一次性 UAC 提权写入并跨重启持久生效
func ConfigureSystemDNSForSuffix(suffix string, listenIP string, port int, allowElevation bool, forcePrompt bool) error {
	if port != 53 {
		log.Printf("[DNS] 警告: Windows NRPT 固定查询 UDP 53 端口（当前: %d）", port)
	}

	ns := NormalizeManagedSuffix(suffix)
	ip := normalizeLoopbackDNSIP(listenIP)

	if IsSystemDNSConfigured(ns, ip) {
		log.Printf("[DNS] Windows NRPT 策略已就绪: %s -> %s:53", ns, ip)
		return nil
	}

	suffixes := []string{ns}
	if ns != ".beagle" {
		suffixes = append(suffixes, ".beagle")
	}
	script := buildNRPTUpsertScript(suffixes, ip)

	// 1. 先尝试直接执行（若当前进程已具备管理员权限可静默成功）
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-EncodedCommand", encodeCommandForPowerShell(script))
	cmd.SysProcAttr = hiddenProcAttr
	if _, err := cmd.CombinedOutput(); err == nil && IsSystemDNSConfigured(ns, ip) {
		log.Printf("[DNS] Windows NRPT 规则已直接写入: %s -> %s:53", ns, ip)
		return nil
	}

	// 2. 若允许一次性 UAC 提权（且非单测进程），拉起 ShellExecuteExW("runas") 持久化注册
	if !allowElevation || isGoTestProcess() {
		return fmt.Errorf("NRPT 规则 %s -> %s 尚未配置（需要管理员授权）", ns, ip)
	}

	promptKey := ns + "@" + ip
	nrptPromptMu.Lock()
	alreadyPrompted := nrptPromptedKeySet[promptKey]
	if alreadyPrompted && !forcePrompt {
		nrptPromptMu.Unlock()
		return fmt.Errorf("NRPT 规则 %s -> %s 尚未启用", ns, ip)
	}
	nrptPromptedKeySet[promptKey] = true
	nrptPromptMu.Unlock()

	if err := runPowerShellElevated(script); err != nil {
		return fmt.Errorf("提权写入 NRPT 规则失败: %w", err)
	}
	if !IsSystemDNSConfigured(ns, ip) {
		return fmt.Errorf("提权完成后未检测到生效的 NRPT 规则 (%s -> %s)", ns, ip)
	}

	log.Printf("[DNS] Windows NRPT 持久化规则已通过一次性授权生效: %s -> %s:53", ns, ip)
	return nil
}

// CleanupSystemDNS 在 Windows 上保持空操作，使 NRPT 后缀规则跨重启持久保留，彻底避免每次启动重复弹出 UAC
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
	var b strings.Builder
	b.WriteString("$ProgressPreference = 'SilentlyContinue'\n")
	for _, s := range suffixes {
		ns := NormalizeManagedSuffix(s)
		fmt.Fprintf(&b, "Get-DnsClientNrptRule -ErrorAction SilentlyContinue | Where-Object { $_.Namespace -contains '%s' -or $_.Namespace -eq '%s' } | Remove-DnsClientNrptRule -Force -ErrorAction SilentlyContinue\n", ns, ns)
		fmt.Fprintf(&b, "Add-DnsClientNrptRule -Namespace '%s' -NameServers '%s' -ErrorAction Stop | Out-Null\n", ns, ip)
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
