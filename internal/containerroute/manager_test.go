package containerroute

import (
	"testing"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/client"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/proxy"
)

type fakeAllocator struct {
	released []string
}

func (*fakeAllocator) Allocate(string) (string, error) { return "127.1.0.1", nil }
func (a *fakeAllocator) Release(domain string)         { a.released = append(a.released, domain) }

type fakeProxy struct {
	started []proxy.Target
	stopped []string
}

func (p *fakeProxy) StartProxy(target proxy.Target) error {
	p.started = append(p.started, target)
	return nil
}
func (p *fakeProxy) StopProxy(vip string, port int) {
	p.stopped = append(p.stopped, vip+":22")
}

func TestSyncReplacesChangedRevisionAndRemovesRevokedRoute(t *testing.T) {
	allocator := &fakeAllocator{}
	proxyManager := &fakeProxy{}
	manager := NewManager(allocator, proxyManager)
	resource := &client.ResourceInfo{
		Type: "container_ssh", ResourceID: "resource-a", Domain: "resource-a.container.beagle",
		AgentIP: "100.64.0.22", ListenPort: 50200, TargetRevision: 3,
	}
	if err := manager.Sync([]*client.ResourceInfo{resource}); err != nil {
		t.Fatal(err)
	}
	if len(proxyManager.started) != 1 || proxyManager.started[0].RemoteAddr != "100.64.0.22:50200" || proxyManager.started[0].Port != 22 {
		t.Fatalf("unexpected initial route: %#v", proxyManager.started)
	}
	if err := manager.Sync([]*client.ResourceInfo{resource}); err != nil {
		t.Fatal(err)
	}
	if len(proxyManager.started) != 1 {
		t.Fatal("unchanged revision must not restart proxy")
	}

	changed := *resource
	changed.TargetRevision = 4
	if err := manager.Sync([]*client.ResourceInfo{&changed}); err != nil {
		t.Fatal(err)
	}
	if len(proxyManager.started) != 2 || len(proxyManager.stopped) != 1 {
		t.Fatal("changed revision must replace proxy")
	}
	if err := manager.Sync(nil); err != nil {
		t.Fatal(err)
	}
	if len(proxyManager.stopped) != 2 || len(allocator.released) != 1 {
		t.Fatal("revoked resource must stop proxy and release DNS mapping")
	}
}
