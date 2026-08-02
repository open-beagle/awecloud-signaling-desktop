package client

import (
	"testing"

	pb "github.com/open-beagle/awecloud-signaling-desktop/pkg/proto"
)

func TestClearResourceCachesKeepsDeviceCredentialAndDropsAuthorizationState(t *testing.T) {
	client := NewDesktopClient("127.0.0.1:1")
	client.desktopID = 42
	client.secret = "device-secret"
	client.authorizedServices = []*pb.AuthorizedService{{Name: "tenant-a-service"}}
	client.cachedHosts = []*HostInfo{{HostName: "tenant-a-host"}}
	client.cachedHostServices["host-a"] = []*pb.AuthorizedService{{Name: "tenant-a-service"}}

	client.ClearResourceCaches()

	if !client.IsAuthenticated() {
		t.Fatal("Tenant switch must preserve the current Desktop device credential")
	}
	if len(client.authorizedServices) != 0 || len(client.cachedHosts) != 0 || len(client.cachedHostServices) != 0 {
		t.Fatalf("authorization-derived cache was not cleared: services=%d hosts=%d host_services=%d",
			len(client.authorizedServices), len(client.cachedHosts), len(client.cachedHostServices))
	}
}
