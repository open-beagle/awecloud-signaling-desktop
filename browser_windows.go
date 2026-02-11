//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// openBrowser 在默认浏览器中打开 URL（Windows）
func openBrowser(url string) error {
	cmd := exec.Command("cmd", "/c", "start", url)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	return cmd.Start()
}
