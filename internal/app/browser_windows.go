//go:build windows

package app

import (
	"os/exec"
	"syscall"
)

// openBrowser 在系统默认浏览器中安全打开 HTTP(S) URL（避免 cmd /c start 截断 & 查询参数）
func openBrowser(rawURL string) error {
	safeURL, err := validateBrowserURL(rawURL)
	if err != nil {
		return err
	}
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", safeURL)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	return cmd.Start()
}
