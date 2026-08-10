//go:build !windows

package main

import (
	"fmt"
	"os"
)

func showStartupError(message string) {
	fmt.Fprintln(os.Stderr, "Beagle Signal startup failed:", message)
}

func showInitialInstallNotice(version string, size int64) {
	fmt.Fprintf(os.Stderr, "Installing Beagle Signal Desktop %s (%.1f MiB)...\n", version, float64(size)/(1024*1024))
}
