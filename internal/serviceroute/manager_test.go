package serviceroute

import (
	"errors"
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
	fail    map[string]error
}

func (p *fakeProxy) StartSVCProxy(target proxy.SVCTarget) error {
	if err := p.fail[target.Domain]; err != nil {
		return err
	}
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

func TestManagerUsesLocalPortWithoutChangingTargetPort(t *testing.T) {
	allocator, proxyManager := &fakeAllocator{}, &fakeProxy{}
	manager := NewManager(allocator, proxyManager)
	resource := &client.ResourceInfo{
		Type: "container_service", TenantID: "tenant-a", ResourceID: "resource-a", Domain: "resource-a.service.beagle",
		AgentIP: "100.64.0.10", SVCProxyPort: 50051, Namespace: "tenant-a", ServiceUID: "service-a",
		ServiceName: "api", PortName: "http", Port: 8090, LocalPort: 18090, Protocol: "TCP", TargetRevision: 2,
		SessionID: "session-a", SourceID: "source-a", TargetRevisionID: "target-a", AuthorizationRevision: 5,
	}
	if err := manager.Sync([]*client.ResourceInfo{resource}); err != nil {
		t.Fatal(err)
	}
	if len(proxyManager.started) != 1 || proxyManager.started[0].Port != 18090 || proxyManager.started[0].TargetPort != 8090 {
		t.Fatalf("unexpected local/target ports: %#v", proxyManager.started)
	}

	resource.LocalPort = 28090
	if err := manager.Sync([]*client.ResourceInfo{resource}); err != nil {
		t.Fatal(err)
	}
	if len(proxyManager.started) != 2 || proxyManager.started[1].Port != 28090 || proxyManager.started[1].TargetPort != 8090 {
		t.Fatalf("changed local port did not preserve target: %#v", proxyManager.started)
	}
}

func TestManagerSkipsFailedListenerAndContinuesOtherRoutes(t *testing.T) {
	allocator := &fakeAllocator{}
	proxyManager := &fakeProxy{fail: map[string]error{"blocked.service.beagle": errors.New("端口被系统保留")}}
	manager := NewManager(allocator, proxyManager)
	blocked := &client.ResourceInfo{
		Type: "container_service", TenantID: "tenant-a", ResourceID: "resource-blocked", Domain: "blocked.service.beagle",
		AgentIP: "100.64.0.10", SVCProxyPort: 50051, Namespace: "tenant-a", ServiceUID: "service-a",
		ServiceName: "blocked", PortName: "http", Port: 8090, Protocol: "TCP", TargetRevision: 2,
		SessionID: "session-blocked", SourceID: "source-a", TargetRevisionID: "target-blocked", AuthorizationRevision: 5,
	}
	available := &client.ResourceInfo{
		Type: "container_service", TenantID: "tenant-a", ResourceID: "resource-available", Domain: "available.service.beagle",
		AgentIP: "100.64.0.10", SVCProxyPort: 50051, Namespace: "tenant-a", ServiceUID: "service-b",
		ServiceName: "available", PortName: "https", Port: 8443, Protocol: "TCP", TargetRevision: 2,
		SessionID: "session-available", SourceID: "source-a", TargetRevisionID: "target-available", AuthorizationRevision: 5,
	}

	err := manager.Sync([]*client.ResourceInfo{blocked, available})
	if err == nil {
		t.Fatal("expected the failed listener to be reported")
	}
	if blocked.LocalError == "" {
		t.Fatal("failed resource was not annotated for the UI")
	}
	if available.LocalError != "" || len(proxyManager.started) != 1 || proxyManager.started[0].Domain != available.Domain {
		t.Fatalf("available route was not preserved: error=%q started=%#v", available.LocalError, proxyManager.started)
	}
	if len(allocator.released) != 1 || allocator.released[0] != blocked.Domain {
		t.Fatalf("failed route allocation was not released: %#v", allocator.released)
	}
	if _, exists := manager.routes[blocked.Domain]; exists {
		t.Fatal("failed route was registered")
	}

	delete(proxyManager.fail, blocked.Domain)
	if err := manager.Sync([]*client.ResourceInfo{blocked, available}); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if blocked.LocalError != "" {
		t.Fatalf("retry did not clear local error: %q", blocked.LocalError)
	}
	if _, exists := manager.routes[blocked.Domain]; !exists {
		t.Fatal("successful retry did not register route")
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
