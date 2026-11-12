package netutil

import (
	"net"
	"regexp"
	"strings"
	"sync/atomic"
)

const DefaultDomainSuffix = ".beagle"

var (
	dnsLabelRegexp     = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	activeDomainSuffix atomic.Value
)

func init() {
	activeDomainSuffix.Store(DefaultDomainSuffix)
}

// NormalizeDomainSuffix 规范化域名根后缀（如 ".acme"），非法值回退 ".beagle"
func NormalizeDomainSuffix(raw string) string {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	trimmed = strings.TrimSuffix(trimmed, ".")
	trimmed = strings.TrimPrefix(trimmed, ".")
	if trimmed == "" || !dnsLabelRegexp.MatchString(trimmed) {
		return DefaultDomainSuffix
	}
	return "." + trimmed
}

// DeriveBrandDomainSuffix 从登录服务器 URL 或 host:port 动态提取品牌二级标签作为内网根后缀
// 例如 https://signal.acme.io -> .acme，IP / localhost / 集群内网域名回退 .beagle
func DeriveBrandDomainSuffix(rawURL string) string {
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(ExtractHost(rawURL)), "."))
	if host == "" || host == "localhost" || net.ParseIP(host) != nil {
		return DefaultDomainSuffix
	}
	for _, internalSuffix := range []string{".localhost", ".local", ".internal", ".cluster.local", ".svc"} {
		if strings.HasSuffix(host, internalSuffix) {
			return DefaultDomainSuffix
		}
	}

	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return DefaultDomainSuffix
	}

	candidate := labels[len(labels)-2]
	if len(labels) >= 3 && len(labels[len(labels)-1]) == 2 && isCompoundSecondLevelTLD(labels[len(labels)-2]) {
		candidate = labels[len(labels)-3]
	}
	if !dnsLabelRegexp.MatchString(candidate) {
		return DefaultDomainSuffix
	}
	return "." + candidate
}

func isCompoundSecondLevelTLD(label string) bool {
	switch label {
	case "com", "net", "org", "gov", "edu", "co", "ac":
		return true
	default:
		return false
	}
}

// SetActiveDomainSuffix 更新当前会话生效的零信任根域名后缀
func SetActiveDomainSuffix(suffix string) string {
	normalized := NormalizeDomainSuffix(suffix)
	activeDomainSuffix.Store(normalized)
	return normalized
}

// ActiveDomainSuffix 获取当前会话生效的零信任根域名后缀
func ActiveDomainSuffix() string {
	if v := activeDomainSuffix.Load(); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return DefaultDomainSuffix
}

// HasManagedDomainSuffix 判断主机名是否属于受管零信任内网后缀（含默认 .beagle 与当前动态品牌后缀）
func HasManagedDomainSuffix(host string) bool {
	cleanHost := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if cleanHost == "" {
		return false
	}
	if strings.HasSuffix(cleanHost, DefaultDomainSuffix) {
		return true
	}
	active := ActiveDomainSuffix()
	return active != "" && strings.HasSuffix(cleanHost, active)
}
