package app

import (
	"testing"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/client"
)

func TestApplyTenantResourcesRejectsCrossTenantSnapshot(t *testing.T) {
	app := &App{activeTenantID: "tenant-a"}
	err := app.applyTenantResources("tenant-a", []*client.ResourceInfo{{
		Type: "container_ssh", TenantID: "tenant-b", Domain: "resource.container.beagle",
	}})
	if err == nil {
		t.Fatal("cross-Tenant resource snapshot must fail closed")
	}
}

func TestTenantResourceDomainsDropsEmptyEntries(t *testing.T) {
	domains := tenantResourceDomains([]*client.ResourceInfo{
		nil,
		{TenantID: "tenant-a"},
		{TenantID: "tenant-a", Domain: "ssh.container.beagle"},
	})
	if len(domains) != 1 {
		t.Fatalf("unexpected domain allowlist: %#v", domains)
	}
	if _, ok := domains["ssh.container.beagle"]; !ok {
		t.Fatal("authorized Tenant domain is missing")
	}
}

func TestSwitchResourceTenantClearsSnapshotWithoutFetching(t *testing.T) {
	app := &App{
		desktopClient:  client.NewDesktopClient("127.0.0.1:1"),
		activeTenantID: "tenant-a",
		tenantResources: []*client.ResourceInfo{{
			Type: "container_service", TenantID: "tenant-a", ResourceID: "service-a",
		}},
		allowedTenantDomains: map[string]struct{}{"service.ns.agent.beagle": {}},
	}

	resources, err := app.SwitchResourceTenant("tenant-b")
	if err != nil {
		t.Fatalf("switch Tenant: %v", err)
	}
	if len(resources) != 0 {
		t.Fatalf("Tenant switch returned resources without an explicit fetch: %#v", resources)
	}
	if app.activeTenantID != "tenant-b" || len(app.tenantResources) != 0 || len(app.allowedTenantDomains) != 0 {
		t.Fatalf("Tenant switch did not clear the previous snapshot: %#v", app)
	}
}
