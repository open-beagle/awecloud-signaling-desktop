//go:build windows

package main

import (
	"os/exec"
)

// openBrowser 在默认浏览器中打开 URL（Windows）
func openBrowser(url string) error {
	cmd := exec.Command("cmd", "/c", "start", url)
	return cmd.Start()
}
