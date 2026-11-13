package dns

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	ManagedHostsBeginMarker = "# BEGIN Beagle Signal Managed Domains"
	ManagedHostsEndMarker   = "# END Beagle Signal Managed Domains"
)

var (
	managedSuffixLabelRegexp = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	managedFQDNRegexp        = regexp.MustCompile(`^_?[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)
	proxifierExclusionRe     = regexp.MustCompile(`(?is)(<ExclusionList\b[^>]*>)(.*?)(</ExclusionList>)`)
	proxifierRuleRe          = regexp.MustCompile(`(?is)<Rule\b[^>]*>.*?</Rule>`)
	proxifierTargetsRe       = regexp.MustCompile(`(?is)(<Targets\b[^>]*>)(.*?)(</Targets>)`)
	proxifierNameRe          = regexp.MustCompile(`(?is)<Name\b[^>]*>\s*(.*?)\s*</Name>`)
)

// ManagedHostEntry 系统 hosts 受管确定性域名与本地回环 VIP 映射项
type ManagedHostEntry struct {
	Domain string
	IP     string
}

// NormalizeManagedSuffix 规范化并严格校验受管 DNS 根后缀（防注入），非法时回退 ".beagle"
func NormalizeManagedSuffix(suffix string) string {
	trimmed := strings.ToLower(strings.TrimSpace(suffix))
	trimmed = strings.TrimSuffix(trimmed, ".")
	trimmed = strings.TrimPrefix(trimmed, ".")
	if trimmed == "" || !managedSuffixLabelRegexp.MatchString(trimmed) {
		return ".beagle"
	}
	return "." + trimmed
}

// VerifySystemDNSResolution 通过操作系统原生解析器（Windows GetAddrInfoW / macOS / Linux）端到端验证动态后缀解析是否可达本地 DNS 服务
func VerifySystemDNSResolution(suffix string) bool {
	canary := CanaryDomainForSuffix(suffix)
	ctx, cancel := context.WithTimeout(context.Background(), 650*time.Millisecond)
	defer cancel()

	resolver := &net.Resolver{PreferGo: false}
	ips, err := resolver.LookupIPAddr(ctx, canary)
	if err != nil {
		return false
	}
	want := net.IPv4(127, 0, 0, 1)
	for _, item := range ips {
		if item.IP != nil && item.IP.Equal(want) {
			return true
		}
	}
	return false
}

// SanitizeManagedHostEntries 校验、去重并按域名排序受管 hosts 映射项（仅允许合法域名与 127.x.x.x 回环地址，彻底杜绝注入）
func SanitizeManagedHostEntries(entries []ManagedHostEntry) []ManagedHostEntry {
	byDomain := make(map[string]string, len(entries))
	for _, raw := range entries {
		domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw.Domain), "."))
		if domain == "" || len(domain) > 253 || !managedFQDNRegexp.MatchString(domain) {
			continue
		}
		ip := net.ParseIP(strings.TrimSpace(raw.IP))
		if ip == nil {
			continue
		}
		ip4 := ip.To4()
		if ip4 == nil || ip4[0] != 127 {
			continue
		}
		byDomain[domain] = ip4.String()
	}
	if len(byDomain) == 0 {
		return nil
	}
	domains := make([]string, 0, len(byDomain))
	for d := range byDomain {
		domains = append(domains, d)
	}
	sort.Strings(domains)
	out := make([]ManagedHostEntry, 0, len(domains))
	for _, d := range domains {
		out = append(out, ManagedHostEntry{Domain: d, IP: byDomain[d]})
	}
	return out
}

// UpsertManagedHostsBlock 在系统 hosts 文本中幂等替换或插入 Beagle Signal 受管确定性域名映射块
func UpsertManagedHostsBlock(existing string, entries []ManagedHostEntry) (string, bool) {
	clean := SanitizeManagedHostEntries(entries)
	eol := "\n"
	if strings.Contains(existing, "\r\n") || (existing == "" && runtime.GOOS == "windows") {
		eol = "\r\n"
	}

	normalized := strings.ReplaceAll(existing, "\r\n", "\n")
	rawLines := strings.Split(normalized, "\n")

	var kept []string
	inManagedBlock := false
	for _, line := range rawLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == ManagedHostsBeginMarker {
			inManagedBlock = true
			continue
		}
		if trimmed == ManagedHostsEndMarker {
			inManagedBlock = false
			continue
		}
		if !inManagedBlock {
			kept = append(kept, line)
		}
	}

	for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}

	var b strings.Builder
	if len(kept) > 0 {
		b.WriteString(strings.Join(kept, eol))
		b.WriteString(eol)
	}
	if len(clean) > 0 {
		if b.Len() > 0 {
			b.WriteString(eol)
		}
		b.WriteString(ManagedHostsBeginMarker)
		b.WriteString(eol)
		for _, item := range clean {
			b.WriteString(item.IP)
			b.WriteByte('\t')
			b.WriteString(item.Domain)
			b.WriteString(eol)
		}
		b.WriteString(ManagedHostsEndMarker)
		b.WriteString(eol)
	}

	updated := b.String()
	return updated, updated != existing
}

// HasManagedHostsEntries 检查 hosts 文本中是否已完整包含全部目标受管域名及其精确 VIP 映射
func HasManagedHostsEntries(hostsContent string, entries []ManagedHostEntry) bool {
	clean := SanitizeManagedHostEntries(entries)
	if len(clean) == 0 {
		return false
	}

	activeMap := make(map[string]string)
	for _, rawLine := range strings.Split(strings.ReplaceAll(hostsContent, "\r\n", "\n"), "\n") {
		line := rawLine
		if hashIdx := strings.IndexByte(line, '#'); hashIdx >= 0 {
			line = line[:hashIdx]
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		ip := net.ParseIP(fields[0])
		if ip == nil || ip.To4() == nil {
			continue
		}
		ipStr := ip.To4().String()
		for _, host := range fields[1:] {
			activeMap[strings.ToLower(strings.TrimSuffix(host, "."))] = ipStr
		}
	}

	for _, item := range clean {
		if activeMap[item.Domain] != item.IP {
			return false
		}
	}
	return true
}

// PatchProxifierProfileXML 针对启用了代理 DNS 解析 (ViaProxy=true) 的 Proxifier 配置文件幂等补齐动态品牌后缀排除与 VIP 本地直连规则
func PatchProxifierProfileXML(rawXML string, suffix string) (string, bool) {
	if !strings.Contains(strings.ToLower(rawXML), "<proxifierprofile") || !strings.Contains(strings.ToLower(rawXML), `<viaproxy enabled="true"`) {
		return rawXML, false
	}

	ns := NormalizeManagedSuffix(suffix)
	label := strings.TrimPrefix(ns, ".")
	wildcardSuffix := "*." + label

	updated := proxifierExclusionRe.ReplaceAllStringFunc(rawXML, func(match string) string {
		sub := proxifierExclusionRe.FindStringSubmatch(match)
		if len(sub) != 4 {
			return match
		}
		body := appendProxifierTargets(sub[2], []string{wildcardSuffix, "*.beagle"}, true)
		return sub[1] + body + sub[3]
	})

	hasGlobalLoopbackWildcard := strings.Contains(strings.ToLower(updated), "127.*.*.*")

	updated = proxifierRuleRe.ReplaceAllStringFunc(updated, func(ruleBlock string) string {
		lowerBlock := strings.ToLower(ruleBlock)
		if !strings.Contains(lowerBlock, `<action type="direct"`) {
			return ruleBlock
		}
		nameMatch := proxifierNameRe.FindStringSubmatch(ruleBlock)
		ruleName := ""
		if len(nameMatch) >= 2 {
			ruleName = strings.ToLower(strings.TrimSpace(nameMatch[1]))
		}

		isLocalhostRule := ruleName == "localhost"
		isBeagleDirectRule := strings.Contains(ruleName, "beagle") || strings.Contains(lowerBlock, "*.beagle")
		if !isLocalhostRule && !isBeagleDirectRule {
			return ruleBlock
		}

		var additions []string
		if isLocalhostRule {
			if !hasGlobalLoopbackWildcard && !strings.Contains(lowerBlock, "127.*.*.*") {
				additions = []string{"127.1.*.*", "127.0.0.2"}
			}
		} else if isBeagleDirectRule {
			additions = []string{wildcardSuffix, label}
			if !hasGlobalLoopbackWildcard {
				additions = append(additions, "127.1.*.*")
			}
		}
		if len(additions) == 0 {
			return ruleBlock
		}

		return proxifierTargetsRe.ReplaceAllStringFunc(ruleBlock, func(tMatch string) string {
			tSub := proxifierTargetsRe.FindStringSubmatch(tMatch)
			if len(tSub) != 4 {
				return tMatch
			}
			return tSub[1] + appendProxifierTargets(tSub[2], additions, false) + tSub[3]
		})
	})

	return updated, updated != rawXML
}

func appendProxifierTargets(rawList string, required []string, keepTrailingSemicolon bool) string {
	trimmed := strings.TrimSpace(rawList)
	hadTrailing := strings.HasSuffix(trimmed, ";")
	var tokens []string
	seen := make(map[string]bool)
	for _, part := range strings.Split(rawList, ";") {
		tok := strings.TrimSpace(part)
		if tok == "" {
			continue
		}
		lower := strings.ToLower(tok)
		if !seen[lower] {
			seen[lower] = true
			tokens = append(tokens, tok)
		}
	}
	changed := false
	for _, req := range required {
		reqTrim := strings.TrimSpace(req)
		if reqTrim == "" {
			continue
		}
		lower := strings.ToLower(reqTrim)
		if !seen[lower] {
			seen[lower] = true
			tokens = append(tokens, reqTrim)
			changed = true
		}
	}
	if !changed {
		return rawList
	}
	joined := strings.Join(tokens, "; ")
	if hadTrailing || keepTrailingSemicolon {
		joined += ";"
	}
	return joined
}

// EnsureProxifierSuffixBypassInDir 扫描指定 Proxifier 配置目录并幂等补齐动态品牌后缀与本地 VIP 直连豁免（首次修改自动生成 .signal.bak 备份）
func EnsureProxifierSuffixBypassInDir(profilesDir string, suffix string) (bool, error) {
	if strings.TrimSpace(profilesDir) == "" {
		return false, nil
	}
	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	anyChanged := false
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".ppx") {
			continue
		}
		fullPath := filepath.Join(profilesDir, entry.Name())
		rawBytes, readErr := os.ReadFile(fullPath)
		if readErr != nil {
			continue
		}
		updatedXML, changed := PatchProxifierProfileXML(string(rawBytes), suffix)
		if !changed {
			continue
		}

		bakPath := fullPath + ".signal.bak"
		if _, statErr := os.Stat(bakPath); os.IsNotExist(statErr) {
			_ = os.WriteFile(bakPath, rawBytes, 0600)
		}
		tmpPath := fullPath + ".signal.tmp"
		if writeErr := os.WriteFile(tmpPath, []byte(updatedXML), 0600); writeErr != nil {
			return anyChanged, writeErr
		}
		if renameErr := os.Rename(tmpPath, fullPath); renameErr != nil {
			_ = os.Remove(tmpPath)
			if writeErr := os.WriteFile(fullPath, []byte(updatedXML), 0600); writeErr != nil {
				return anyChanged, writeErr
			}
		}
		anyChanged = true
	}
	return anyChanged, nil
}
