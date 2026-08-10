//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

func showStartupError(message string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	messageBox := user32.NewProc("MessageBoxW")
	title, _ := syscall.UTF16PtrFromString("Beagle Signal 启动失败")
	body, _ := syscall.UTF16PtrFromString(message + "\n\n详细信息见 logs\\launcher.log")
	_, _, _ = messageBox.Call(0, uintptr(unsafe.Pointer(body)), uintptr(unsafe.Pointer(title)), 0x10)
}

func showInitialInstallNotice(version string, size int64) {
	user32 := syscall.NewLazyDLL("user32.dll")
	messageBox := user32.NewProc("MessageBoxW")
	title, _ := syscall.UTF16PtrFromString("Beagle Signal 首次启动")
	bodyText := fmt.Sprintf("即将下载并安装 Desktop %s（%.1f MB）。\n\n点击“确定”后开始；完成后将自动打开客户端。", version, float64(size)/(1024*1024))
	body, _ := syscall.UTF16PtrFromString(bodyText)
	_, _, _ = messageBox.Call(0, uintptr(unsafe.Pointer(body)), uintptr(unsafe.Pointer(title)), 0x40)
}
