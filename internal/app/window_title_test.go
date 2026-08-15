package app

import "testing"

func TestFormatWindowBuildTimeOmitsTimezone(t *testing.T) {
	got := formatWindowBuildTime("2026-08-15T23:40:42+08:00")
	if want := "2026-08-15T23:40:42"; got != want {
		t.Fatalf("formatWindowBuildTime() = %q, want %q", got, want)
	}
}

func TestFormatWindowBuildTimePreservesUnknownValue(t *testing.T) {
	got := formatWindowBuildTime("unknown")
	if want := "unknown"; got != want {
		t.Fatalf("formatWindowBuildTime() = %q, want %q", got, want)
	}
}
