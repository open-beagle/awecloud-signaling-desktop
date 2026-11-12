package tailscale

import (
	"net/netip"
	"strings"
	"testing"

	"tailscale.com/tailcfg"
)

func TestEndpointSummaryUsesMapAndOmitsIdentityFields(t *testing.T) {
	n := &tailcfg.Node{
		Name:      "private-name-must-not-log",
		Endpoints: []netip.AddrPort{netip.MustParseAddrPort("203.0.113.8:41641")},
		HomeDERP:  900,
	}
	s := peerEndpointSummary(n.View())
	for _, want := range []string{"source=control-map", "203.0.113.8:41641", "homeDERP=900", "discoKeyPresent=false"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %s: %s", want, s)
		}
	}
	if strings.Contains(s, n.Name) {
		t.Fatal("unselected identity field leaked")
	}
}
