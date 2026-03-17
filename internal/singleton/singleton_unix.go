//go:build !windows

package singleton

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

var lockFile *os.File

// lockFilePath 锁文件路径
// 使用用户配置目录而非临时目录，避免以下问题：
// 1. macOS 的 os.TempDir() 在不同启动方式下可能返回不同路径
// 2. Linux 的 /tmp 可能被 systemd-tmpfiles 定期清理
func lockFilePath() string {
	// 优先使用用户配置目录（与应用数据目录一致）
	configDir, err := os.UserConfigDir()
	if err == nil {
		dir := filepath.Join(configDir, "signaling-desktop")
		os.MkdirAll(dir, 0755)
		return filepath.Join(dir, "singleton.lock")
	}

	// 回退到用户主目录
	homeDir, err := os.UserHomeDir()
	if err == nil {
		dir := filepath.Join(homeDir, ".signaling-desktop")
		os.MkdirAll(dir, 0755)
		return filepath.Join(dir, "singleton.lock")
	}

	// 最终回退到临时目录
	return filepath.Join(os.TempDir(), "awecloud-signaling-desktop.lock")
}

// CheckSingleInstance 检查是否已有实例运行
// 返回 true 表示当前是唯一实例，可以继续运行
// 返回 false 表示已有实例运行，应该退出
func CheckSingleInstance() bool {
	lockPath := lockFilePath()

	// 尝试打开或创建锁文件
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return false
	}

	// 尝试获取排他锁（非阻塞）
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		// 无法获取锁，说明已有实例运行
		f.Close()
		return false
	}

	// 写入当前进程 PID
	f.Truncate(0)
	f.Seek(0, 0)
	f.WriteString(strconv.Itoa(os.Getpid()))

	lockFile = f
	return true
}

// ReleaseSingleInstance 释放单实例锁
func ReleaseSingleInstance() {
	if lockFile != nil {
		syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		lockFile.Close()
		os.Remove(lockFilePath())
		lockFile = nil
	}
}

// GetErrorMessage 获取错误信息（用于显示给用户）
func GetErrorMessage() string {
	return fmt.Sprintf("应用已在运行中")
}
