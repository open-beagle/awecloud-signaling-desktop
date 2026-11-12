//go:build !windows

package app

import (
	"os/exec"
	"runtime"
)

// openBrowser 在系统默认浏览器中安全打开 HTTP(S) URL（Unix/Linux/macOS）
func openBrowser(rawURL string) error {
	safeURL, err := validateBrowserURL(rawURL)
	if err != nil {
		return err
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", safeURL)
	default:
		cmd = exec.Command("xdg-open", safeURL)
	}
	return cmd.Start()
}
