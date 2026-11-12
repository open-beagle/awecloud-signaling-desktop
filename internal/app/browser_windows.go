//go:build windows

package app

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	shell32       = syscall.NewLazyDLL("shell32.dll")
	shellExecuteW = shell32.NewProc("ShellExecuteW")
)

const swShowNormal = 1

// openBrowser 通过 Win32 ShellExecuteW 直接调起系统默认浏览器并前置显示（保留完整查询参数且不派生隐藏控制台）
func openBrowser(rawURL string) error {
	safeURL, err := validateBrowserURL(rawURL)
	if err != nil {
		return err
	}
	verbPtr, err := syscall.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	urlPtr, err := syscall.UTF16PtrFromString(safeURL)
	if err != nil {
		return err
	}
	ret, _, callErr := shellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(urlPtr)),
		0,
		0,
		swShowNormal,
	)
	// Win32 ShellExecuteW 返回值 > 32 表示成功
	if ret <= 32 {
		return fmt.Errorf("ShellExecuteW failed (code=%d): %w", ret, callErr)
	}
	return nil
}
