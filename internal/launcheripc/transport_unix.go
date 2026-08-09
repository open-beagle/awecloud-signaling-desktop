//go:build linux || darwin

package launcheripc

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
)

func listenLocal(endpoint string) (net.Listener, func() error, error) {
	dir := filepath.Dir(endpoint)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, nil, fmt.Errorf("create socket parent dir failed: %w", err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, nil, fmt.Errorf("chmod socket parent dir failed: %w", err)
	}

	// Remove stale socket if not listening
	_ = os.Remove(endpoint)

	listener, err := net.Listen("unix", endpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("listen unix socket failed: %w", err)
	}

	if err := os.Chmod(endpoint, 0600); err != nil {
		listener.Close()
		return nil, nil, fmt.Errorf("chmod socket file failed: %w", err)
	}

	cleanup := func() error {
		_ = listener.Close()
		return os.Remove(endpoint)
	}

	return listener, cleanup, nil
}

func dialLocal(ctx context.Context, endpoint string) (net.Conn, error) {
	var dialer net.Dialer
	return dialer.DialContext(ctx, "unix", endpoint)
}

func verifyLocalPeer(conn net.Conn, expectedUID uint32) error {
	peerUID, err := localPeerUID(conn)
	if err != nil {
		return err
	}
	if peerUID != expectedUID {
		return fmt.Errorf("peer UID %d does not match expected UID %d", peerUID, expectedUID)
	}
	return nil
}

func controlUnixSocket(conn net.Conn, control func(int) error) error {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return fmt.Errorf("connection is not a Unix domain socket")
	}

	rawConn, err := unixConn.SyscallConn()
	if err != nil {
		return fmt.Errorf("get raw socket connection failed: %w", err)
	}

	var controlErr error
	err = rawConn.Control(func(fd uintptr) {
		controlErr = control(int(fd))
	})
	if err != nil {
		return err
	}
	return controlErr
}
