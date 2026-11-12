package dns

import (
	"regexp"
	"strings"
)

var managedSuffixLabelRegexp = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

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
