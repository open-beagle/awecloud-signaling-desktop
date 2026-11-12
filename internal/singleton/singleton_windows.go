//go:build windows

package singleton

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	kernel32      = syscall.NewLazyDLL("kernel32.dll")
	user32        = syscall.NewLazyDLL("user32.dll")
	createMutex   = kernel32.NewProc("CreateMutexW")
	releaseMutex  = kernel32.NewProc("ReleaseMutex")
	closeHandle   = kernel32.NewProc("CloseHandle")
	findWindow    = user32.NewProc("FindWindowW")
	setForeground = user32.NewProc("SetForegroundWindow")
	showWindow    = user32.NewProc("ShowWindow")
	isIconic      = user32.NewProc("IsIconic")
)

const (
	ERROR_ALREADY_EXISTS = 183
	SW_RESTORE           = 9
	SW_SHOW              = 5
)

var mutexHandle uintptr

// mutexName 互斥锁名称
const mutexName = "Global\\AWECloudSignalingDesktop"

// windowClassName Wails 窗口类名
const windowClassName = "wails"

func resolvedMutexName() (string, bool) {
	if customDir := strings.TrimSpace(os.Getenv("SIGNAL_DESKTOP_APP_DIR")); customDir != "" {
		canonical := strings.ToLower(filepath.Clean(customDir))
		sum := sha256.Sum256([]byte(canonical))
		return fmt.Sprintf("%s_%x", mutexName, sum[:8]), true
	}
	return mutexName, false
}

// CheckSingleInstance 检查是否已有实例运行
// 返回 true 表示当前是唯一实例，可以继续运行
// 返回 false 表示已有实例运行，应该退出
func CheckSingleInstance() bool {
	targetMutex, isolated := resolvedMutexName()
	// 将字符串转换为 UTF-16
	name, _ := syscall.UTF16PtrFromString(targetMutex)

	// 创建互斥锁
	// createMutex.Call 的第三个返回值就是 GetLastError 的结果
	// 不能单独调用 GetLastError，因为 .Call() 本身会重置 last error
	handle, _, err := createMutex.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if handle == 0 {
		return false
	}

	// 检查是否已存在
	if err == syscall.Errno(ERROR_ALREADY_EXISTS) {
		// 默认模式下尝试激活已有窗口；隔离目录模式下避免误拉起日常客户端窗口
		if !isolated {
			activateExistingWindow()
		}
		closeHandle.Call(handle)
		return false
	}

	// 保存句柄，程序退出时释放
	mutexHandle = handle
	return true
}

// ReleaseSingleInstance 释放单实例锁
func ReleaseSingleInstance() {
	if mutexHandle != 0 {
		releaseMutex.Call(mutexHandle)
		closeHandle.Call(mutexHandle)
		mutexHandle = 0
	}
}

// activateExistingWindow 激活已有窗口
func activateExistingWindow() {
	// 查找 Wails 窗口
	className, _ := syscall.UTF16PtrFromString(windowClassName)
	hwnd, _, _ := findWindow.Call(uintptr(unsafe.Pointer(className)), 0)

	if hwnd != 0 {
		// 检查窗口是否最小化
		iconic, _, _ := isIconic.Call(hwnd)
		if iconic != 0 {
			// 恢复窗口
			showWindow.Call(hwnd, SW_RESTORE)
		} else {
			// 显示窗口
			showWindow.Call(hwnd, SW_SHOW)
		}
		// 激活窗口
		setForeground.Call(hwnd)
	}
}

// GetErrorMessage 获取错误信息（用于显示给用户）
func GetErrorMessage() string {
	return fmt.Sprintf("应用已在运行中")
}
