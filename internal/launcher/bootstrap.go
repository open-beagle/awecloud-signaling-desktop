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
	hasCurrent := err == nil
	if hasCurrent {
		if err := migrateCurrentAppName(paths, current); err != nil {
			return nil, fmt.Errorf("migrate current Desktop App entry failed: %w", err)
		}
		// 自动更新已关闭。Launcher 只负责校验并启动当前版本；升级必须由
		// Desktop 中的手动更新流程显式触发。
		logger.Printf("automatic update disabled; using installed Desktop App version %s sha256 %s", current.Version, current.Artifact.SHA256)
		return current, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		logger.Printf("installed Desktop App is not usable: %v", err)
	}

	serverURL, err := normalizeServerURL(serverAddress)
	if err != nil {
		return nil, fmt.Errorf("invalid Launcher server address: %w", err)
	}
	logger.Printf("requesting public Desktop manifest from %s", serverURL.String())
	manifest, err := FetchPublicManifest(ctx, serverAddress)
	if err != nil {
		return nil, fmt.Errorf("fetch Desktop manifest failed: %w", err)
	}
	artifact := manifest.Artifacts.App
	logger.Printf("installing Desktop App version %s (%d bytes)", manifest.Release.Version, artifact.Size)
	if !hasCurrent && onInitialInstall != nil {
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

	appName, err := installVersionedArtifact(paths, result.PartPath, manifest.Release.Version, artifact.SHA256)
	if err != nil {
		return nil, fmt.Errorf("install Desktop App failed: %w", err)
	}

	current = &CurrentInfo{
		SchemaVersion: 1,
		Version:       manifest.Release.Version,
		CommitID:      manifest.Release.CommitID,
		CommitTime:    manifest.Release.PublishedAt,
		App:           appName,
		Artifact:      *artifact,
		InstalledAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := atomicWriteJSON(paths.CurrentFile, current); err != nil {
		return nil, fmt.Errorf("write initial current.json failed: %w", err)
	}
	logger.Printf("Desktop App version %s sha256 %s installed successfully", current.Version, current.Artifact.SHA256)
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
	logicalName := paths.LogicalAppName(version)
	archiveName := paths.ArtifactAppName(version, current.Artifact.SHA256)
	if (current.App != logicalName && current.App != archiveName) || filepath.Base(current.App) != current.App {
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

// installVersionedArtifact keeps an immutable SHA-addressed copy for rollback and
// exposes the active build through the stable, human-readable version name.
func installVersionedArtifact(paths *Paths, partPath, version, artifactSHA string) (string, error) {
	archivePath := filepath.Join(paths.VersionsDir, paths.ArtifactAppName(version, artifactSHA))
	if err := os.Remove(archivePath); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(partPath)
		return "", err
	}
	if err := os.Rename(partPath, archivePath); err != nil {
		_ = os.Remove(partPath)
		return "", err
	}
	if err := os.Chmod(archivePath, 0700); err != nil {
		return "", err
	}

	appName := paths.LogicalAppName(version)
	if err := copyFileAtomically(archivePath, filepath.Join(paths.VersionsDir, appName)); err != nil {
		return "", err
	}
	return appName, nil
}

func migrateCurrentAppName(paths *Paths, current *CurrentInfo) error {
	logicalName := paths.LogicalAppName(current.Version)
	if current.App == logicalName {
		return nil
	}
	archiveName := paths.ArtifactAppName(current.Version, current.Artifact.SHA256)
	if current.App != archiveName {
		return errors.New("current.json contains an invalid app name")
	}
	if err := copyFileAtomically(filepath.Join(paths.VersionsDir, archiveName), filepath.Join(paths.VersionsDir, logicalName)); err != nil {
		return err
	}
	current.App = logicalName
	return atomicWriteJSON(paths.CurrentFile, current)
}

func copyFileAtomically(sourcePath, destinationPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	temporary, err := os.CreateTemp(filepath.Dir(destinationPath), ".app-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := io.Copy(temporary, source); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(0700); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Remove(destinationPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(temporaryPath, destinationPath)
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
