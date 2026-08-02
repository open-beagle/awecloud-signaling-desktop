package config

import (
	"path/filepath"
	"testing"
)

func useTestConfigRoot(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("APPDATA", root)
	t.Setenv("XDG_CONFIG_HOME", root)
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
