package autostart

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/launcheripc"
)

const AppAutostartKey = "BeagleSignalDesktop"

// ResolveTargetExecutable 解析登录自启动应注册的可执行文件绝对路径：
// 优先采用 Launcher 注入的 BEAGLE_SIGNAL_LAUNCHER_PATH，确保客户端静默自更新后自启入口永不失效；
// 独立运行或开发模式下回退至当前进程可执行文件。
func ResolveTargetExecutable() (string, error) {
	if fromEnv := strings.TrimSpace(os.Getenv(launcheripc.EnvLauncherPath)); fromEnv != "" {
		if info, err := os.Stat(fromEnv); err == nil && !info.IsDir() {
			if abs, absErr := filepath.Abs(fromEnv); absErr == nil {
				return abs, nil
			}
			return fromEnv, nil
		}
	}

	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("获取当前程序路径失败: %w", err)
	}
	if resolved, evalErr := filepath.EvalSymlinks(exePath); evalErr == nil {
		exePath = resolved
	}
	if abs, absErr := filepath.Abs(exePath); absErr == nil {
		exePath = abs
	}

	parentDir := filepath.Dir(exePath)
	if strings.EqualFold(filepath.Base(parentDir), "versions") {
		rootDir := filepath.Dir(parentDir)
		candidates := []string{"beagle-signal"}
		if runtime.GOOS == "windows" {
			candidates = []string{"beagle-signal.exe", "signaling-desktop.exe"}
		}
		for _, name := range candidates {
			cand := filepath.Join(rootDir, name)
			if info, statErr := os.Stat(cand); statErr == nil && !info.IsDir() {
				return cand, nil
			}
		}
	}

	return exePath, nil
}

// IsEnabled 查询当前操作系统用户是否已启用 Beagle Signal 登录自启动
func IsEnabled() bool {
	return platformIsEnabled()
}

// SetEnabled 开启或关闭当前操作系统用户的登录自启动
func SetEnabled(enabled bool) error {
	if !enabled {
		return platformDisable()
	}
	targetExe, err := ResolveTargetExecutable()
	if err != nil {
		return err
	}
	return platformEnable(targetExe)
}
