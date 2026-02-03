//go:build !windows

package main

import (
	"os/exec"
	"runtime"
)

// openBrowser 在默认浏览器中打开 URL（Unix/Linux/macOS）
func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		// Linux 和其他 Unix 系统
		cmd = exec.Command("xdg-open", url)
	}

	return cmd.Start()
}
