//go:build !windows

package launcheripc

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func NewEndpoint() (string, error) {
	suffix, err := randomEndpointSuffix()
	if err != nil {
		return "", err
	}

	baseDir := os.Getenv("XDG_RUNTIME_DIR")
	if baseDir == "" || runtime.GOOS == "darwin" {
		baseDir = os.TempDir()
	}
	runtimeDir := filepath.Join(baseDir, fmt.Sprintf("beagle-signal-%d", os.Getuid()))
	if err := os.MkdirAll(runtimeDir, 0700); err != nil {
		return "", fmt.Errorf("create Launcher runtime directory failed: %w", err)
	}
	if err := os.Chmod(runtimeDir, 0700); err != nil {
		return "", fmt.Errorf("secure Launcher runtime directory failed: %w", err)
	}
	endpoint := filepath.Join(runtimeDir, "l-"+suffix+".sock")
	if len(endpoint) > 96 {
		return "", fmt.Errorf("Launcher endpoint path is too long")
	}
	return endpoint, nil
}

func ExpectedPeerUID() uint32 {
	return uint32(os.Getuid())
}
