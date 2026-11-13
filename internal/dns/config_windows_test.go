//go:build windows

package dns

import (
	"os"
	"path/filepath"
	"strings"
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

func TestUpsertManagedHostsBlockAndValidation(t *testing.T) {
	existing := "# Copyright Corp\r\n127.0.0.1       localhost\r\n192.168.1.200   studio.example.internal\r\n"
	entries := []ManagedHostEntry{
		{Domain: "home.alice.acme.", IP: "127.1.42.19"},
		{Domain: "WORK.ALICE.ACME", IP: "127.1.88.7"},
		// 非法注入项与非 127 网段必须被过滤丢弃
		{Domain: "bad;rm -rf.acme", IP: "127.1.0.5"},
		{Domain: "evil.alice.acme", IP: "8.8.8.8"},
	}

	updated, changed := UpsertManagedHostsBlock(existing, entries)
	require.True(t, changed)
	require.Contains(t, updated, "192.168.1.200   studio.example.internal")
	require.Contains(t, updated, "127.1.42.19\thome.alice.acme")
	require.Contains(t, updated, "127.1.88.7\twork.alice.acme")
	require.NotContains(t, updated, "bad;rm")
	require.NotContains(t, updated, "8.8.8.8")
	require.True(t, HasManagedHostsEntries(updated, entries[:2]))

	// 再次幂等执行不应发生任何变更
	updated2, changed2 := UpsertManagedHostsBlock(updated, entries)
	require.False(t, changed2)
	require.Equal(t, updated, updated2)

	// 更新单个设备的 VIP 后旧块被干净替换
	entriesUpdated := []ManagedHostEntry{
		{Domain: "home.alice.acme", IP: "127.1.42.20"},
	}
	updated3, changed3 := UpsertManagedHostsBlock(updated2, entriesUpdated)
	require.True(t, changed3)
	require.Equal(t, 1, strings.Count(updated3, ManagedHostsBeginMarker))
	require.False(t, HasManagedHostsEntries(updated3, entries[:2]))
	require.True(t, HasManagedHostsEntries(updated3, entriesUpdated))
}

func TestEnsureProxifierSuffixBypassIdempotentAndBackup(t *testing.T) {
	tempDir := t.TempDir()
	samplePPX := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<ProxifierProfile version="102" platform="Windows" product_id="0" product_minver="400">
	<Options>
		<Resolve>
			<AutoModeDetection enabled="false" />
			<ViaProxy enabled="true" />
			<ExclusionList OnlyFromListMode="false">%ComputerName%; localhost; *.local; *.beagle; *.test;</ExclusionList>
		</Resolve>
	</Options>
	<RuleList>
		<Rule enabled="true">
			<Action type="Direct" />
			<Targets>localhost; 127.0.0.1; %ComputerName%; ::1</Targets>
			<Name>Localhost</Name>
		</Rule>
		<Rule enabled="true">
			<Action type="Direct" />
			<Targets>*.beagle; beagle</Targets>
			<Name>Beagle Direct</Name>
		</Rule>
	</RuleList>
</ProxifierProfile>`

	ppxPath := filepath.Join(tempDir, "Default.ppx")
	require.NoError(t, os.WriteFile(ppxPath, []byte(samplePPX), 0600))

	changed, err := EnsureProxifierSuffixBypassInDir(tempDir, ".acme")
	require.NoError(t, err)
	require.True(t, changed)

	// 验证自动生成了 .signal.bak 原始备份
	bakBytes, err := os.ReadFile(ppxPath + ".signal.bak")
	require.NoError(t, err)
	require.Equal(t, samplePPX, string(bakBytes))

	patchedBytes, err := os.ReadFile(ppxPath)
	require.NoError(t, err)
	patched := string(patchedBytes)
	require.Contains(t, patched, "*.acme")
	require.Contains(t, patched, "127.1.*.*")
	require.Contains(t, patched, "127.0.0.2")

	// 二次执行严格幂等无改动
	changedAgain, err := EnsureProxifierSuffixBypassInDir(tempDir, "acme")
	require.NoError(t, err)
	require.False(t, changedAgain)

	// 验证已按规范配置 127.*.*.* 与双域名直连的 Profile 保持零漂移（changed=false）
	alreadyCompliantPPX := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<ProxifierProfile version="102" platform="Windows" product_id="0" product_minver="400">
	<Options>
		<Resolve>
			<ViaProxy enabled="true" />
			<ExclusionList OnlyFromListMode="false">%ComputerName%; localhost; *.local; *.beagle; *.acme; acme; *.test;</ExclusionList>
		</Resolve>
	</Options>
	<RuleList>
		<Rule enabled="true">
			<Action type="Direct" />
			<Targets>localhost; 127.0.0.1; 127.*.*.*; %ComputerName%; ::1</Targets>
			<Name>Localhost</Name>
		</Rule>
		<Rule enabled="true">
			<Action type="Direct" />
			<Targets>*.beagle; beagle; *.acme; acme; 100.64.*.*</Targets>
			<Name>Beagle &amp; Acme Direct</Name>
		</Rule>
	</RuleList>
</ProxifierProfile>`
	outCompliant, compliantChanged := PatchProxifierProfileXML(alreadyCompliantPPX, ".acme")
	require.False(t, compliantChanged)
	require.Equal(t, alreadyCompliantPPX, outCompliant)

	// 验证生成的 NRPT+hosts PowerShell 脚本包含 Base64 受管块与缓存刷新
	script := buildNRPTAndHostsUpsertScript([]string{".acme", ".beagle"}, "127.0.0.2", []ManagedHostEntry{
		{Domain: "home.alice.acme", IP: "127.1.10.20"},
	})
	require.Contains(t, script, "Add-DnsClientNrptRule -Namespace '.acme'")
	require.Contains(t, script, "System32\\drivers\\etc\\hosts")
	require.Contains(t, script, "Clear-DnsClientCache")
}
