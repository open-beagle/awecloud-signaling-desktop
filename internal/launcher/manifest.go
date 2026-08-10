package launcher

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/launcheripc"
)

const maxManifestSize = 1024 * 1024

type ManifestRelease struct {
	Version string `json:"version"`
	Channel string `json:"channel"`
}

type ManifestArtifacts struct {
	App *launcheripc.ArtifactPayload `json:"app"`
}

type PublicManifest struct {
	SchemaVersion int               `json:"schema_version"`
	GeneratedAt   string            `json:"generated_at"`
	ExpiresAt     string            `json:"expires_at"`
	Release       ManifestRelease   `json:"release"`
	Artifacts     ManifestArtifacts `json:"artifacts"`
}

func FetchPublicManifest(ctx context.Context, serverAddress, currentVersion string) (*PublicManifest, error) {
	baseURL, err := normalizeServerURL(serverAddress)
	if err != nil {
		return nil, err
	}

	manifestURL := *baseURL
	manifestURL.Path = strings.TrimRight(manifestURL.Path, "/") + "/api/v1/public/updater/manifest"
	query := manifestURL.Query()
	query.Set("component", "desktop")
	query.Set("os", runtime.GOOS)
	query.Set("arch", runtime.GOARCH)
	query.Set("channel", "stable")
	if currentVersion != "" {
		query.Set("current_version", strings.TrimPrefix(currentVersion, "v"))
	}
	manifestURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create manifest request failed: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request public manifest failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("public manifest HTTP status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxManifestSize+1))
	if err != nil {
		return nil, fmt.Errorf("read public manifest failed: %w", err)
	}
	if len(body) > maxManifestSize {
		return nil, errors.New("public manifest exceeds 1 MiB")
	}

	var manifest PublicManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("decode public manifest failed: %w", err)
	}
	if err := validateManifest(&manifest, baseURL); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func validateManifest(manifest *PublicManifest, serverURL *url.URL) error {
	if manifest.SchemaVersion != 1 {
		return fmt.Errorf("unsupported manifest schema_version %d", manifest.SchemaVersion)
	}
	version, err := normalizeVersion(manifest.Release.Version)
	if err != nil {
		return err
	}
	manifest.Release.Version = version

	expiresAt, err := time.Parse(time.RFC3339, manifest.ExpiresAt)
	if err != nil {
		return fmt.Errorf("invalid manifest expires_at: %w", err)
	}
	if !expiresAt.After(time.Now().UTC()) {
		return errors.New("public manifest has expired")
	}

	artifact := manifest.Artifacts.App
	if artifact == nil {
		return errors.New("public manifest is missing artifacts.app")
	}
	if artifact.Role != "app" || artifact.OS != runtime.GOOS || artifact.Arch != runtime.GOARCH {
		return fmt.Errorf("manifest app artifact does not match %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if artifact.PackageType != "binary" && runtime.GOOS != "darwin" {
		return fmt.Errorf("unsupported app package_type %q", artifact.PackageType)
	}
	if artifact.ID == "" || artifact.Size <= 0 {
		return errors.New("manifest app artifact has invalid id or size")
	}
	if len(artifact.SHA256) != 64 {
		return errors.New("manifest app artifact has invalid sha256")
	}
	if _, err := hex.DecodeString(artifact.SHA256); err != nil {
		return errors.New("manifest app artifact has invalid sha256")
	}
	artifact.SHA256 = strings.ToLower(artifact.SHA256)

	downloadURL, err := serverURL.Parse(artifact.DownloadURL)
	if err != nil || downloadURL.Host == "" {
		return errors.New("manifest app artifact has invalid download_url")
	}
	if err := validateNetworkURL(downloadURL); err != nil {
		return fmt.Errorf("invalid artifact download_url: %w", err)
	}
	artifact.DownloadURL = downloadURL.String()
	return nil
}

func normalizeServerURL(address string) (*url.URL, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, errors.New("server address is empty")
	}
	if !strings.Contains(address, "://") {
		address = "https://" + address
	}
	parsed, err := url.Parse(address)
	if err != nil || parsed.Host == "" {
		return nil, errors.New("server address is invalid")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("server address must be an HTTP(S) origin")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if parsed.Path != "" {
		return nil, errors.New("server address must not contain a path")
	}
	if err := validateNetworkURL(parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

func validateNetworkURL(parsed *url.URL) error {
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme != "http" {
		return errors.New("URL scheme must be HTTPS")
	}
	host := parsed.Hostname()
	if !strings.EqualFold(host, "localhost") {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return errors.New("HTTP is only allowed for loopback development servers")
		}
	}
	return nil
}

func normalizeVersion(version string) (string, error) {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid release version %q", version)
	}
	for _, part := range parts {
		if part == "" {
			return "", fmt.Errorf("invalid release version %q", version)
		}
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				return "", fmt.Errorf("invalid release version %q", version)
			}
		}
	}
	return version, nil
}
