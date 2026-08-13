package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteLocalKubeconfigCreatesAndReplaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".kube", "config")
	if err := writeLocalKubeconfig(path, []byte("apiVersion: v1\nkind: Config\n")); err != nil {
		t.Fatal(err)
	}
	if err := writeLocalKubeconfig(path, []byte("apiVersion: v1\nkind: Config\ncurrent-context: beijing.beagle\n")); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "apiVersion: v1\nkind: Config\ncurrent-context: beijing.beagle\n" {
		t.Fatalf("unexpected content: %q", content)
	}
}

func TestDecodeWindowsCommandOutputUTF16(t *testing.T) {
	input := []byte{0xff, 0xfe, 'U', 0, 'b', 0, 'u', 0, 'n', 0, 't', 0, 'u', 0, '\r', 0, '\n', 0}
	if actual := decodeWindowsCommandOutput(input); actual != "Ubuntu\r\n" {
		t.Fatalf("unexpected output: %q", actual)
	}
}

func TestWSLSystemDistrosAreExcluded(t *testing.T) {
	if !isWSLSystemDistro("docker-desktop") || !isWSLSystemDistro("Docker-Desktop-Data") {
		t.Fatal("expected Docker Desktop distributions to be excluded")
	}
	if isWSLSystemDistro("Ubuntu-24.04") {
		t.Fatal("user distribution must remain available")
	}
}
