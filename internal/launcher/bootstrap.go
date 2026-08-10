package launcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func EnsureCurrentApp(ctx context.Context, paths *Paths, serverAddress string, logger *log.Logger, onInitialInstall func(version string, size int64)) (*CurrentInfo, error) {
	current, err := loadValidCurrent(paths)
	if err == nil {
		logger.Printf("using installed Desktop App version %s", current.Version)
		return current, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		logger.Printf("installed Desktop App is not usable: %v", err)
	}

	serverURL, err := normalizeServerURL(serverAddress)
	if err != nil {
		return nil, fmt.Errorf("invalid Launcher server address: %w", err)
	}
	logger.Printf("no usable Desktop App found; requesting public manifest from %s", serverURL.String())
	manifest, err := FetchPublicManifest(ctx, serverAddress, "")
	if err != nil {
		return nil, fmt.Errorf("fetch initial Desktop manifest failed: %w", err)
	}
	artifact := manifest.Artifacts.App
	logger.Printf("installing Desktop App version %s (%d bytes)", manifest.Release.Version, artifact.Size)
	if onInitialInstall != nil {
		onInitialInstall(manifest.Release.Version, artifact.Size)
	}

	lastLoggedProgress := -10
	result, err := DownloadAndVerifyArtifact(ctx, paths.DownloadsDir, artifact, func(progress int) {
		if progress == 100 || progress >= lastLoggedProgress+10 {
			logger.Printf("Desktop App download progress: %d%%", progress)
			lastLoggedProgress = progress
		}
	})
	if err != nil {
		return nil, fmt.Errorf("download initial Desktop App failed: %w", err)
	}

	appName := paths.LogicalAppName(manifest.Release.Version)
	appPath := filepath.Join(paths.VersionsDir, appName)
	if err := os.Remove(appPath); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(result.PartPath)
		return nil, fmt.Errorf("remove invalid Desktop App failed: %w", err)
	}
	if err := os.Rename(result.PartPath, appPath); err != nil {
		_ = os.Remove(result.PartPath)
		return nil, fmt.Errorf("install Desktop App failed: %w", err)
	}
	if err := os.Chmod(appPath, 0700); err != nil {
		return nil, fmt.Errorf("set Desktop App permissions failed: %w", err)
	}

	current = &CurrentInfo{
		SchemaVersion: 1,
		Version:       manifest.Release.Version,
		App:           appName,
		Artifact:      *artifact,
		InstalledAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := atomicWriteJSON(paths.CurrentFile, current); err != nil {
		return nil, fmt.Errorf("write initial current.json failed: %w", err)
	}
	logger.Printf("Desktop App version %s installed successfully", current.Version)
	return current, nil
}

func loadValidCurrent(paths *Paths) (*CurrentInfo, error) {
	var current CurrentInfo
	if err := readStateJSON(paths.CurrentFile, &current); err != nil {
		return nil, err
	}
	if current.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported current.json schema_version %d", current.SchemaVersion)
	}
	version, err := normalizeVersion(current.Version)
	if err != nil {
		return nil, err
	}
	current.Version = version
	expectedName := paths.LogicalAppName(version)
	if current.App != expectedName || filepath.Base(current.App) != current.App {
		return nil, errors.New("current.json contains an invalid app name")
	}
	appPath := filepath.Join(paths.VersionsDir, current.App)
	info, err := os.Lstat(appPath)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("current Desktop App is not a regular file")
	}
	if len(current.Artifact.SHA256) == 64 {
		digest, err := fileSHA256(appPath)
		if err != nil {
			return nil, err
		}
		if !strings.EqualFold(digest, current.Artifact.SHA256) {
			return nil, errors.New("current Desktop App sha256 mismatch")
		}
	}
	return &current, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
