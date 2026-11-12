package diagnostics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotationRetainsBoundedRecentLogs(t *testing.T) {
	p := filepath.Join(t.TempDir(), "desktop.log")
	for _, line := range []string{"123456\n", "abcdef\n", "uvwxyz\n"} {
		if err := appendBounded(p, []byte(line), 10); err != nil {
			t.Fatal(err)
		}
	}
	current, _ := os.ReadFile(p)
	previous, _ := os.ReadFile(p + ".1")
	if string(current) != "uvwxyz\n" || string(previous) != "abcdef\n" {
		t.Fatalf("unexpected retained logs: %q %q", current, previous)
	}
	if err := appendBounded(p, []byte(strings.Repeat("x", 100)), 10); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(p)
	if st.Size() > 10 {
		t.Fatal("oversized record exceeded cap")
	}
}

func TestCredentialsAreRemovedBeforePersistence(t *testing.T) {
	s := redact(`DeviceToken=private-value secret="private password" Authorization: Bearer private-bearer url=https://example.com/?code=private-code&state=private-state endpoint=203.0.113.1:3479`)
	for _, value := range []string{"private-value", "private password", "private-code", "private-state", "private-bearer"} {
		if strings.Contains(s, value) {
			t.Fatal("credential survived redaction")
		}
	}
	if !strings.Contains(s, "203.0.113.1:3479") {
		t.Fatal("diagnostic endpoint lost")
	}
}

func TestTunnelDiagnosticsPersistWithoutUI(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { sink.Lock(); sink.dir = ""; sink.Unlock() }()
	Tunnel("magicsock: disco: peer probe timeout")
	Tunnel("control: LoginInteractive secret=do-not-record")
	b, err := os.ReadFile(filepath.Join(dir, "logs", "network.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "peer probe timeout") || strings.Contains(string(b), "LoginInteractive") {
		t.Fatal("incorrect network diagnostic selection")
	}
}
