//go:build windows

package autostart

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const windowsRunKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

func platformIsEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, windowsRunKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()

	val, _, err := k.GetStringValue(AppAutostartKey)
	return err == nil && strings.TrimSpace(val) != ""
}

func platformEnable(targetExe string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, windowsRunKeyPath, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return fmt.Errorf("打开当前用户自启动注册表失败: %w", err)
	}
	defer k.Close()

	quoted := fmt.Sprintf(`"%s"`, targetExe)
	if err := k.SetStringValue(AppAutostartKey, quoted); err != nil {
		return fmt.Errorf("写入登录自启动项失败: %w", err)
	}
	return nil
}

func platformDisable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, windowsRunKeyPath, registry.SET_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("打开当前用户自启动注册表失败: %w", err)
	}
	defer k.Close()

	if err := k.DeleteValue(AppAutostartKey); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("移除登录自启动项失败: %w", err)
	}
	return nil
}
