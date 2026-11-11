//go:build linux

package autostart

import (
	"fmt"
	"os"
	"path/filepath"
)

func linuxDesktopEntryPath() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "", err
		}
		cfgDir = filepath.Join(home, ".config")
	}
	return filepath.Join(cfgDir, "autostart", "beagle-signal-desktop.desktop"), nil
}

func platformIsEnabled() bool {
	entryPath, err := linuxDesktopEntryPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(entryPath)
	return err == nil
}

func platformEnable(targetExe string) error {
	entryPath, err := linuxDesktopEntryPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(entryPath), 0755); err != nil {
		return err
	}
	content := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=Beagle Signal Desktop
Exec="%s"
Terminal=false
X-GNOME-Autostart-enabled=true
`, targetExe)
	return os.WriteFile(entryPath, []byte(content), 0644)
}

func platformDisable() error {
	entryPath, err := linuxDesktopEntryPath()
	if err != nil {
		return err
	}
	if err := os.Remove(entryPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
