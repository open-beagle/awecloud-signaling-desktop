package config

import (
	"os"
	"path/filepath"
	"testing"
)

func useTestConfigRoot(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("SIGNAL_DESKTOP_APP_DIR", "")
	t.Setenv("APPDATA", root)
	t.Setenv("XDG_CONFIG_HOME", root)
}

func TestGetAppDirHonorsSignalDesktopAppDirEnv(t *testing.T) {
	defaultRoot := t.TempDir()
	isolatedDir := filepath.Join(t.TempDir(), "signal-staging")
	t.Setenv("APPDATA", defaultRoot)
	t.Setenv("XDG_CONFIG_HOME", defaultRoot)
	t.Setenv("SIGNAL_DESKTOP_APP_DIR", "  "+isolatedDir+"  ")

	appDir, err := GetAppDir()
	if err != nil {
		t.Fatalf("GetAppDir: %v", err)
	}
	if appDir != filepath.Clean(isolatedDir) {
		t.Fatalf("GetAppDir = %q, want %q", appDir, filepath.Clean(isolatedDir))
	}

	cfgPath, err := GetConfigPath()
	if err != nil {
		t.Fatalf("GetConfigPath: %v", err)
	}
	if cfgPath != filepath.Join(isolatedDir, "desktop.json") {
		t.Fatalf("GetConfigPath = %q, want %q", cfgPath, filepath.Join(isolatedDir, "desktop.json"))
	}

	tunnelDir, err := GetTunnelStateDir()
	if err != nil {
		t.Fatalf("GetTunnelStateDir: %v", err)
	}
	if tunnelDir != filepath.Join(isolatedDir, "tunnel") {
		t.Fatalf("GetTunnelStateDir = %q, want %q", tunnelDir, filepath.Join(isolatedDir, "tunnel"))
	}

	stored := &Config{ServerAddress: "https://signal.example.com", ClientID: "alice"}
	if err := stored.Save(); err != nil {
		t.Fatalf("Save isolated config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(defaultRoot, "signaling-desktop", "desktop.json")); !os.IsNotExist(err) {
		t.Fatalf("default user config dir must remain untouched when SIGNAL_DESKTOP_APP_DIR is set")
	}
}

func TestLoadDropsCredentialsFromDifferentPinnedServer(t *testing.T) {
	useTestConfigRoot(t)
	original := buildAddress
	buildAddress = "https://ztna.example.com"
	t.Cleanup(func() { buildAddress = original })

	stored := &Config{ServerAddress: "https://signal.example.com", ClientID: "user", DeviceToken: "64:secret", RememberMe: true}
	if err := stored.Save(); err != nil {
		t.Fatalf("save fixture: %v", err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.ServerAddress != buildAddress || loaded.ClientID != "user" {
		t.Fatalf("unexpected migrated config: %#v", loaded)
	}
	if loaded.DeviceToken != "" || loaded.RememberMe {
		t.Fatal("credentials from a different Server must not survive migration")
	}
}

func TestLoadKeepsCredentialsForEquivalentPinnedServer(t *testing.T) {
	useTestConfigRoot(t)
	original := buildAddress
	buildAddress = "https://ztna.example.com/"
	t.Cleanup(func() { buildAddress = original })

	stored := &Config{ServerAddress: "https://ZTNA.example.com", ClientID: "user", DeviceToken: "64:secret", RememberMe: true}
	if err := stored.Save(); err != nil {
		t.Fatalf("save fixture: %v", err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.DeviceToken != "64:secret" || !loaded.RememberMe {
		t.Fatal("same-Server credentials should remain available")
	}
}

func TestSelectServerClearsBoundCredentials(t *testing.T) {
	config := &Config{ServerAddress: "https://signal.example.com", ClientID: "user", DeviceToken: "64:secret", RememberMe: true}
	if !config.SelectServer("https://ztna.example.com") {
		t.Fatal("server change was not detected")
	}
	if config.DeviceToken != "" || config.RememberMe {
		t.Fatal("server change must clear bound credentials")
	}
	if filepath.Clean(config.ServerAddress) == filepath.Clean("https://signal.example.com") {
		t.Fatal("new Server address was not selected")
	}
}

func TestPortPreferencesPersistOnCurrentDevice(t *testing.T) {
	useTestConfigRoot(t)
	stored := &Config{
		ServerAddress:   "https://signal.example.com",
		PortPreferences: map[string]int{"tenant-a:resource-a": 18090},
	}
	if err := stored.Save(); err != nil {
		t.Fatalf("save port preference: %v", err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("load port preference: %v", err)
	}
	if got := loaded.PortPreferences["tenant-a:resource-a"]; got != 18090 {
		t.Fatalf("local port preference = %d, want 18090", got)
	}
}
