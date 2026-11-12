package tailscale

import (
	"context"
	"fmt"
	"time"

	"tailscale.com/client/local"
	"tailscale.com/ipn"
	"tailscale.com/tailcfg"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/diagnostics"
)

// Record only selected public routing fields, never serialize the whole map.
func peerEndpointSummary(peer tailcfg.NodeView) string {
	return fmt.Sprintf("[PeerEndpoints] node=%s peer=%v source=control-map endpoints=%v homeDERP=%d discoKeyPresent=%t", peer.Key().ShortString(), peer.Addresses().AsSlice(), peer.Endpoints().AsSlice(), peer.HomeDERP(), !peer.DiscoKey().IsZero())
}

func watchDiscoveryDiagnostics(ctx context.Context, lc *local.Client) {
	debugCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	err := lc.SetComponentDebugLogging(debugCtx, "magicsock", 24*time.Hour)
	cancel()
	if err != nil {
		diagnostics.Record("network", fmt.Sprintf("[DiscoveryDiagnostics] magicsock enable failed: %v", err))
	} else {
		diagnostics.Record("network", "[DiscoveryDiagnostics] magicsock=true duration=24h; ping/pong/call-me-maybe/timeouts enabled")
	}
	for ctx.Err() == nil {
		watcher, err := lc.WatchIPNBus(ctx, ipn.NotifyInitialNetMap|ipn.NotifyNoPrivateKeys)
		if err == nil {
			for {
				n, readErr := watcher.Next()
				if readErr != nil {
					err = readErr
					break
				}
				if n.NetMap == nil {
					continue
				}
				diagnostics.Record("network", fmt.Sprintf("[NetworkMap] peers=%d", len(n.NetMap.Peers)))
				for _, peer := range n.NetMap.Peers {
					if peer.Valid() {
						diagnostics.Record("network", peerEndpointSummary(peer))
					}
				}
			}
			watcher.Close()
		}
		if ctx.Err() != nil {
			return
		}
		diagnostics.Record("network", fmt.Sprintf("[DiscoveryDiagnostics] map watch interrupted, retrying: %v", err))
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

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
	if st.Self != nil {
		diagnostics.Record("network", fmt.Sprintf("[SelfEndpoints] endpoints=%v homeDERP=%s", st.Self.Addrs, st.Self.Relay))
	}
	for _, peer := range st.Peer {
		if peer == nil {
			continue
		}
		diagnostics.Record("network", fmt.Sprintf("[PeerSnapshot] peer=%v online=%t active=%t direct=%t current=%s peerRelay=%s homeDERP=%s rx=%d tx=%d lastHandshake=%s", peer.TailscaleIPs, peer.Online, peer.Active, peer.CurAddr != "", peer.CurAddr, peer.PeerRelay, peer.Relay, peer.RxBytes, peer.TxBytes, peer.LastHandshake.Format(time.RFC3339)))
	}
}
