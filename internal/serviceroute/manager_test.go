package serviceroute

import (
	"testing"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/client"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/proxy"
)

type fakeAllocator struct{ released []string }

func (a *fakeAllocator) Allocate(domain string) (string, error) { return "127.1.0.8", nil }
func (a *fakeAllocator) Release(domain string)                  { a.released = append(a.released, domain) }

type fakeProxy struct {
	started []proxy.SVCTarget
	stopped []string
}

func (p *fakeProxy) StartSVCProxy(target proxy.SVCTarget) error {
	p.started = append(p.started, target)
	return nil
}
func (p *fakeProxy) StopSVCProxy(vip string, port int) { p.stopped = append(p.stopped, vip) }

func TestManagerBindsAndRevokesV2ServiceRoute(t *testing.T) {
	allocator, proxyManager := &fakeAllocator{}, &fakeProxy{}
	manager := NewManager(allocator, proxyManager)
	resource := &client.ResourceInfo{
		Type: "container_service", TenantID: "tenant-a", ResourceID: "resource-a", Domain: "resource-a.service.beagle",
		AgentIP: "100.64.0.10", SVCProxyPort: 50051, Namespace: "tenant-a", ServiceUID: "service-a",
		ServiceName: "api", PortName: "https", Port: 443, Protocol: "TCP", TargetRevision: 2,
		SessionID: "session-a", SourceID: "source-a", TargetRevisionID: "target-a", AuthorizationRevision: 5,
	}
	if err := manager.Sync([]*client.ResourceInfo{resource}); err != nil {
		t.Fatal(err)
	}
	if len(proxyManager.started) != 1 || proxyManager.started[0].SessionID != "session-a" || proxyManager.started[0].ServiceUID != "service-a" {
		t.Fatalf("unexpected v2 route: %#v", proxyManager.started)
	}
	if err := manager.Sync([]*client.ResourceInfo{resource}); err != nil {
		t.Fatal(err)
	}
	if len(proxyManager.started) != 1 {
		t.Fatalf("unchanged route restarted: %#v", proxyManager.started)
	}

	if err := manager.Sync(nil); err != nil {
		t.Fatal(err)
	}
	if len(proxyManager.stopped) != 1 || len(allocator.released) != 1 || allocator.released[0] != "resource-a.service.beagle" {
		t.Fatalf("route was not revoked: stopped=%v released=%v", proxyManager.stopped, allocator.released)
	}
}

func TestManagerReplacesServiceUIDAndRemovesOneOfMultiplePorts(t *testing.T) {
	allocator, proxyManager := &fakeAllocator{}, &fakeProxy{}
	manager := NewManager(allocator, proxyManager)
	https := &client.ResourceInfo{
		Type: "container_service", TenantID: "tenant-a", ResourceID: "resource-https", Domain: "https.service.beagle",
		AgentIP: "100.64.0.10", SVCProxyPort: 50051, Namespace: "tenant-a", ServiceUID: "service-a",
		ServiceName: "api", PortName: "https", Port: 443, Protocol: "TCP", TargetRevision: 2,
		SessionID: "session-https", SourceID: "source-a", TargetRevisionID: "target-https", AuthorizationRevision: 5,
	}
	metrics := &client.ResourceInfo{
		Type: "container_service", TenantID: "tenant-a", ResourceID: "resource-metrics", Domain: "metrics.service.beagle",
		AgentIP: "100.64.0.10", SVCProxyPort: 50051, Namespace: "tenant-a", ServiceUID: "service-a",
		ServiceName: "api", PortName: "metrics", Port: 9090, Protocol: "TCP", TargetRevision: 2,
		SessionID: "session-metrics", SourceID: "source-a", TargetRevisionID: "target-metrics", AuthorizationRevision: 5,
	}
	if err := manager.Sync([]*client.ResourceInfo{https, metrics}); err != nil {
		t.Fatal(err)
	}
	if len(proxyManager.started) != 2 {
		t.Fatalf("expected two independent port routes, got %#v", proxyManager.started)
	}

	recreatedHTTPS := *https
	recreatedHTTPS.ServiceUID = "service-b"
	if err := manager.Sync([]*client.ResourceInfo{&recreatedHTTPS}); err != nil {
		t.Fatal(err)
	}
	if len(proxyManager.started) != 3 || proxyManager.started[2].ServiceUID != "service-b" {
		t.Fatalf("service UID change did not replace route: %#v", proxyManager.started)
	}
	if len(proxyManager.stopped) != 2 {
		t.Fatalf("expected replaced HTTPS and removed metrics routes to stop, got %#v", proxyManager.stopped)
	}
	if len(allocator.released) != 1 || allocator.released[0] != metrics.Domain {
		t.Fatalf("removed port route did not release only its domain: %#v", allocator.released)
	}
}
