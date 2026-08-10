//go:build windows

package launcheripc

import "fmt"

func NewEndpoint() (string, error) {
	suffix, err := randomEndpointSuffix()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`\\.\pipe\beagle-signal-launcher-%s`, suffix), nil
}

func ExpectedPeerUID() uint32 {
	return 0
}
