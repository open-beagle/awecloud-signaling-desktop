package app

import (
	"fmt"
	"net/url"
	"strings"
)

func validateBrowserURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("empty browser URL")
	}
	if strings.ContainsAny(trimmed, "\r\n\x00") {
		return "", fmt.Errorf("browser URL contains control characters")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid browser URL: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if (scheme != "http" && scheme != "https") || parsed.Host == "" {
		return "", fmt.Errorf("unsupported browser URL scheme or host")
	}
	return parsed.String(), nil
}
