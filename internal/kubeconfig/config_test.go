package kubeconfig

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestMergeCreatesValidSharedUserConfig(t *testing.T) {
	output, err := Merge(nil, []Cluster{
		{Name: "beijing.beagle", Server: "https://kubernetes.wodcloud.com"},
		{Name: "chengdu.beagle", Server: "https://kubernetes.cn-chengdu.bc-cloud.com"},
	}, "beijing.beagle")
	if err != nil {
		t.Fatal(err)
	}

	var cfg Config
	if err := yaml.Unmarshal(output, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.CurrentContext != "beijing.beagle" || len(cfg.Clusters) != 2 || len(cfg.Contexts) != 2 {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if len(cfg.Users) != 1 || cfg.Users[0].Name != UserName || cfg.Users[0].User["token"] != UserToken {
		t.Fatalf("shared user not installed: %#v", cfg.Users)
	}
	for _, context := range cfg.Contexts {
		if context.Context["user"] != UserName {
			t.Fatalf("context %s references missing user", context.Name)
		}
	}
}

func TestMergeOverwritesMatchingEntriesAndPreservesUnrelated(t *testing.T) {
	existing := []byte(`apiVersion: v1
kind: Config
preferences: {}
clusters:
- name: personal
  cluster:
    server: https://personal.example
- name: beijing.beagle
  cluster:
    server: https://old.example
contexts:
- name: personal
  context:
    cluster: personal
    user: personal
- name: beijing.beagle
  context:
    cluster: beijing.beagle
    user: old
current-context: personal
users:
- name: personal
  user:
    token: keep-me
- name: who
  user:
    token: old-token
`)
	output, err := Merge(existing, []Cluster{{Name: "beijing.beagle", Server: "https://kubernetes.wodcloud.com"}}, "beijing.beagle")
	if err != nil {
		t.Fatal(err)
	}
	text := string(output)
	for _, expected := range []string{"https://personal.example", "keep-me", "https://kubernetes.wodcloud.com", "current-context: beijing.beagle"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %q in:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "https://old.example") || strings.Contains(text, "old-token") {
		t.Fatalf("matching entries were not overwritten:\n%s", text)
	}
}

func TestMergeRejectsMalformedExistingConfig(t *testing.T) {
	if _, err := Merge([]byte("clusters: ["), []Cluster{{Name: "x", Server: "https://x"}}, "x"); err == nil {
		t.Fatal("expected malformed config error")
	}
}
