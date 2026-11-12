package tailscale

import (
	"context"
	"fmt"
	"time"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/diagnostics"
)

func (m *Manager) recordPeerDiagnostics() {
	lc, err := m.LocalClient()
	if err != nil || lc == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	st, err := lc.Status(ctx)
	if err != nil {
		diagnostics.Record("network", "[PeerSnapshot] status unavailable")
		return
	}
	if st == nil {
		return
	}
	for _, peer := range st.Peer {
		if peer == nil {
			continue
		}
		diagnostics.Record("network", fmt.Sprintf("[PeerSnapshot] peer=%v online=%t active=%t direct=%t current=%s candidates=%v relay=%s rx=%d tx=%d lastHandshake=%s", peer.TailscaleIPs, peer.Online, peer.Active, peer.CurAddr != "", peer.CurAddr, peer.Addrs, peer.Relay, peer.RxBytes, peer.TxBytes, peer.LastHandshake.Format(time.RFC3339)))
	}
}
