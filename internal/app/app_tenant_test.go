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

func TestAccountDomainNamesIncludesOnlySupportedDomainTypes(t *testing.T) {
	domains := accountDomainNames([]*client.DomainInfo{
		nil,
		{Domain: " BEAGLE-242.BEIJING.BEAGLE. ", Type: "ssh"},
		{Domain: "beijing.beagle", Type: "k8sapi"},
		{Domain: "service.ns.agent.beagle", Type: "container_service"},
		{Type: "ssh"},
	})
	if len(domains) != 2 {
		t.Fatalf("unexpected account domain allowlist: %#v", domains)
	}
	if _, ok := domains["beagle-242.beijing.beagle"]; !ok {
		t.Fatal("SSH domain is missing from account allowlist")
	}
	if _, ok := domains["beijing.beagle"]; !ok {
		t.Fatal("Kubernetes API domain is missing from account allowlist")
	}
}

func TestDomainAuthorizationUsesAccountAndTenantAllowlists(t *testing.T) {
	app := &App{
		activeTenantID:        "tenant-a",
		allowedAccountDomains: map[string]struct{}{"beagle-242.beijing.beagle": {}},
		allowedTenantDomains:  map[string]struct{}{"service.ns.agent.beagle": {}},
	}

	for _, domain := range []string{
		"BEAGLE-242.BEIJING.BEAGLE.",
		"service.ns.agent.beagle",
	} {
		if !app.isDomainAllowed(domain) {
			t.Fatalf("authorized domain was rejected: %s", domain)
		}
	}
	if app.isDomainAllowed("other-tenant.beagle") {
		t.Fatal("unrelated domain was allowed while Tenant scope is active")
	}
}

func TestSwitchResourceTenantClearsSnapshotWithoutFetching(t *testing.T) {
	app := &App{
		desktopClient:  client.NewDesktopClient("127.0.0.1:1"),
		activeTenantID: "tenant-a",
		tenantResources: []*client.ResourceInfo{{
			Type: "container_service", TenantID: "tenant-a", ResourceID: "service-a",
		}},
		allowedTenantDomains:  map[string]struct{}{"service.ns.agent.beagle": {}},
		allowedAccountDomains: map[string]struct{}{"beagle-242.beijing.beagle": {}},
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
	if _, ok := app.allowedAccountDomains["beagle-242.beijing.beagle"]; !ok {
		t.Fatal("Tenant switch cleared the account SSH/Kubernetes API allowlist")
	}
}

func TestClearTenantContextClearsAllDomainAllowlists(t *testing.T) {
	app := &App{
		activeTenantID:        "tenant-a",
		allowedAccountDomains: map[string]struct{}{"beagle-242.beijing.beagle": {}},
		allowedTenantDomains:  map[string]struct{}{"service.ns.agent.beagle": {}},
	}

	app.clearTenantContext()

	if app.activeTenantID != "" || app.allowedAccountDomains != nil || app.allowedTenantDomains != nil {
		t.Fatalf("identity cleanup retained domain authorization state: %#v", app)
	}
}
