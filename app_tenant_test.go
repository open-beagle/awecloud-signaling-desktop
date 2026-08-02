package main

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
