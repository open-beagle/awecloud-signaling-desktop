//go:build darwin

package launcheripc

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

func localPeerUID(conn net.Conn) (uint32, error) {
	var peerUID uint32
	err := controlUnixSocket(conn, func(fd int) error {
		ucred, err := unix.GetsockoptXucred(fd, unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if err != nil {
			return fmt.Errorf("getsockopt LOCAL_PEERCRED failed: %w", err)
		}
		peerUID = ucred.Uid
		return nil
	})
	return peerUID, err
}
