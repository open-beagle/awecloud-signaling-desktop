//go:build windows

package dns

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMatchNRPTRuleEntryAndPowerShellFallbackOutput(t *testing.T) {
	require.True(t, matchNRPTRuleEntry([]string{".acme"}, "127.0.0.2", ".acme", "127.0.0.2"))
	require.True(t, matchNRPTRuleEntry([]string{".beagle", ".ACME"}, "127.0.0.2;127.0.0.3", "acme", "127.0.0.2"))
	require.False(t, matchNRPTRuleEntry([]string{".beagle"}, "127.0.0.2", ".acme", "127.0.0.2"))
	require.False(t, matchNRPTRuleEntry([]string{".acme"}, "127.0.0.3", ".acme", "127.0.0.2"))

	// 复现 Windows PowerShell 5.1 将 System.Net.IPAddress 序列化为 Format-List 文本的场景
	ps51Output := `
Address            : 33554559
AddressFamily      : InterNetwork
ScopeId            :
IsIPv6Multicast    : False
IPAddressToString  : 127.0.0.2
`
	require.True(t, matchNRPTPowerShellOutput(ps51Output, "127.0.0.2"))
	require.True(t, matchNRPTPowerShellOutput("127.0.0.2\r\n", "127.0.0.2"))
	require.False(t, matchNRPTPowerShellOutput(ps51Output, "127.0.0.3"))
}
