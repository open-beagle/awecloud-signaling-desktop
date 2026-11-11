//go:build darwin

package autostart

import (
	"fmt"
	"os"
	"path/filepath"
)

const macOSLaunchAgentLabel = "com.openbeagle.signaling.desktop"

func darwinPlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", macOSLaunchAgentLabel+".plist"), nil
}

func platformIsEnabled() bool {
	plistPath, err := darwinPlistPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(plistPath)
	return err == nil
}

func platformEnable(targetExe string) error {
	plistPath, err := darwinPlistPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(plistPath), 0755); err != nil {
		return err
	}
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
</dict>
</plist>
`, macOSLaunchAgentLabel, targetExe)
	return os.WriteFile(plistPath, []byte(content), 0644)
}

func platformDisable() error {
	plistPath, err := darwinPlistPath()
	if err != nil {
		return err
	}
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
