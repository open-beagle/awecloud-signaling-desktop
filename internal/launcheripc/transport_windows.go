//go:build windows

package launcheripc

import (
	"context"
	"fmt"
	"net"

	"github.com/tailscale/go-winio"
)

func listenLocal(endpoint string) (net.Listener, func() error, error) {
	cfg := winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;SY)(A;;GA;;;OW)",
		MessageMode:        false,
		InputBufferSize:    65536,
		OutputBufferSize:   65536,
	}

	listener, err := winio.ListenPipe(endpoint, &cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("winio ListenPipe failed: %w", err)
	}

	cleanup := func() error {
		return listener.Close()
	}

	return listener, cleanup, nil
}

func dialLocal(ctx context.Context, endpoint string) (net.Conn, error) {
	return winio.DialPipeContext(ctx, endpoint)
}

func verifyLocalPeer(conn net.Conn, expectedUID uint32) error {
	// Tailscale go-winio ListenPipe restricts connection via DACL to current user SID and LocalSystem.
	return nil
}
