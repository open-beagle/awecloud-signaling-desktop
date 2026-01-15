//go:build windows

// embed_windows.go - Windows 平台嵌入资源
package tailscale

import (
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// wintun.dll 在构建时由 scripts/build.sh 自动下载
// 如果文件不存在，go:embed 会报错，需要先运行 scripts/download_wintun.sh
//
//go:embed resources/wintun.dll
var wintunFS embed.FS

// extractWintun 释放嵌入的 wintun.dll 到可执行文件目录
// 返回 dll 的完整路径
func extractWintun() (string, error) {
	// 获取可执行文件目录
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("获取可执行文件路径失败: %w", err)
	}
	exeDir := filepath.Dir(exe)
	dllPath := filepath.Join(exeDir, "wintun.dll")

	// 检查是否已存在
	if _, err := os.Stat(dllPath); err == nil {
		log.Printf("[DEBUG] [Tunnel] wintun.dll 已存在: %s", dllPath)
		return dllPath, nil
	}

	// 从嵌入资源读取
	data, err := wintunFS.ReadFile("resources/wintun.dll")
	if err != nil {
		return "", fmt.Errorf("读取嵌入的 wintun.dll 失败: %w", err)
	}

	// 写入到可执行文件目录
	if err := os.WriteFile(dllPath, data, 0644); err != nil {
		// 如果写入失败（可能是权限问题），尝试写入临时目录
		tmpDir := os.TempDir()
		dllPath = filepath.Join(tmpDir, "awecloud-wintun.dll")
		if err := os.WriteFile(dllPath, data, 0644); err != nil {
			return "", fmt.Errorf("写入 wintun.dll 失败: %w", err)
		}
	}

	log.Printf("[INFO] [Tunnel] wintun.dll 已释放: %s", dllPath)
	return dllPath, nil
}
