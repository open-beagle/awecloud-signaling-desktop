package launcheripc

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

func NewSessionToken() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate Launcher session token failed: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func randomEndpointSuffix() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate Launcher endpoint suffix failed: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}
