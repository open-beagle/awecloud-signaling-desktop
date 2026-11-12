package diagnostics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tailscale.com/envknob"
)

func TestDiscoveryEnablesAlreadyRegisteredKnobAndPersistsEvents(t *testing.T) {
	previous := os.Getenv("TS_DEBUG_DISCO")
	defer envknob.Setenv("TS_DEBUG_DISCO", previous)
	envknob.Setenv("TS_DEBUG_DISCO", "false")
	registered := envknob.RegisterBool("TS_DEBUG_DISCO")
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { sink.Lock(); sink.dir = ""; sink.Unlock() }()
	EnableDiscoveryLogging()
	if !registered() {
		t.Fatal("cached Tailscale knob was not enabled")
	}
	for _, event := range []string{"send ping", "recv pong", "call-me-maybe", "timeout waiting for pong"} {
		Tunnel("[v1] magicsock: disco: " + event)
	}
	b, err := os.ReadFile(filepath.Join(dir, "logs", "network.log"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"schema=2", "send ping", "recv pong", "call-me-maybe", "timeout waiting for pong"} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("missing %q", want)
		}
	}
}
