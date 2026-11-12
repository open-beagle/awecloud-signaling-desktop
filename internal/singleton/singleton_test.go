//go:build windows

package singleton

import (
	"path/filepath"
	"testing"
)

func TestResolvedMutexNameIsolatesBySignalDesktopAppDir(t *testing.T) {
	t.Setenv("SIGNAL_DESKTOP_APP_DIR", "")
	defaultName, isolated := resolvedMutexName()
	if isolated || defaultName != mutexName {
		t.Fatalf("default mutex = (%q, %v), want (%q, false)", defaultName, isolated, mutexName)
	}

	dirA := filepath.Join(t.TempDir(), "staging-a")
	dirB := filepath.Join(t.TempDir(), "staging-b")

	t.Setenv("SIGNAL_DESKTOP_APP_DIR", dirA)
	nameA, isolatedA := resolvedMutexName()
	if !isolatedA || nameA == mutexName {
		t.Fatalf("isolated mutex A = (%q, %v), expected distinct isolated name", nameA, isolatedA)
	}

	t.Setenv("SIGNAL_DESKTOP_APP_DIR", dirB)
	nameB, isolatedB := resolvedMutexName()
	if !isolatedB || nameB == nameA {
		t.Fatalf("distinct app dirs must not share mutex name: A=%q B=%q", nameA, nameB)
	}
}
