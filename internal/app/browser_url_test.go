package app

import (
	"testing"
)

func TestValidateBrowserURLAllowsHTTPSWithQueryAmpersands(t *testing.T) {
	raw := "https://signal.example.com/auth/desktop/44ff8825?prompt=login&hint=alice"
	got, err := validateBrowserURL(raw)
	if err != nil {
		t.Fatalf("expected valid https URL, got err: %v", err)
	}
	if got != raw {
		t.Fatalf("expected URL %q preserved intact, got %q", raw, got)
	}
}

func TestValidateBrowserURLRejectsUnsafeSchemesAndControlChars(t *testing.T) {
	cases := []string{
		"",
		"file:///C:/Windows/System32/calc.exe",
		"javascript:alert(1)",
		"custom://callback",
		"https://signal.example.com/auth\r\nmalicious",
		"https:///missing-host",
	}
	for _, tc := range cases {
		if _, err := validateBrowserURL(tc); err == nil {
			t.Fatalf("expected unsafe URL %q to be rejected", tc)
		}
	}
}
