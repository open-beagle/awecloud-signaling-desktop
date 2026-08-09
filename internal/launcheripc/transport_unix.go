//go:build !windows

package launcheripc

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
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
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return fmt.Errorf("connection is not a Unix domain socket")
	}

	rawConn, err := unixConn.SyscallConn()
	if err != nil {
		return fmt.Errorf("get raw socket connection failed: %w", err)
	}

	var peerUID uint32
	var sysErr error

	err = rawConn.Control(func(fd uintptr) {
		ucred, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if err != nil {
			sysErr = err
			return
		}
		peerUID = ucred.Uid
	})

	if err != nil {
		return err
	}
	if sysErr != nil {
		return fmt.Errorf("getsockopt SO_PEERCRED failed: %w", sysErr)
	}

	if peerUID != expectedUID {
		return fmt.Errorf("peer UID %d does not match expected UID %d", peerUID, expectedUID)
	}
	return nil
}

var _ = syscall.SIGTERM
