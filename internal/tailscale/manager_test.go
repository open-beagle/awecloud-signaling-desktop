package tailscale

import (
	"path/filepath"
	"testing"
)

func TestScopedStateDirIsolatesDesktopServers(t *testing.T) {
	base := filepath.Join(t.TempDir(), "tunnel")
	first := scopedStateDir(base, "https://signal.example.com")
	second := scopedStateDir(base, "https://ztna.example.com")
	if first == second {
		t.Fatal("different Desktop Servers must not share tsnet state")
	}
	if scopedStateDir(base, "https://ZTNA.example.com/") != second {
		t.Fatal("equivalent Server addresses must share the same scoped state")
	}
	if scopedStateDir(base, "") != base {
		t.Fatal("legacy unscoped callers must retain the base state directory")
	}
}
