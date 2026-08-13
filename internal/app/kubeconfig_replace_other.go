//go:build !windows

package app

import "os"

func replaceKubeconfigFile(source, destination string) error {
	return os.Rename(source, destination)
}
