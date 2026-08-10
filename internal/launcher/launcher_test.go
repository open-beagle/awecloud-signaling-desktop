package launcher

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/launcheripc"
	"github.com/stretchr/testify/require"
)

func TestPathsAndLogicalAppName(t *testing.T) {
	tmpDir := t.TempDir()
	paths, err := NewPaths(tmpDir)
	require.NoError(t, err)

	require.Equal(t, filepath.Join(tmpDir, "state", "current.json"), paths.CurrentFile)
	require.Equal(t, filepath.Join(tmpDir, "state", "previous.json"), paths.PreviousFile)
	require.Equal(t, filepath.Join(tmpDir, "logs", "launcher.log"), paths.LauncherLogFile)

	name := paths.LogicalAppName("1.2.3")
	require.Contains(t, name, "beagle-signal-1.2.3.app")
}

func TestInstanceLockRejectsSecondLauncher(t *testing.T) {
	paths, err := NewPaths(t.TempDir())
	require.NoError(t, err)

	first, err := AcquireInstanceLock(paths.LauncherLockFile)
	require.NoError(t, err)
	defer first.Release()

	second, err := AcquireInstanceLock(paths.LauncherLockFile)
	require.ErrorIs(t, err, ErrAlreadyRunning)
	require.Nil(t, second)
}

func TestEnsureCurrentAppDownloadsAndPersistsInitialVersion(t *testing.T) {
	tmpDir := t.TempDir()
	paths, err := NewPaths(tmpDir)
	require.NoError(t, err)

	content := []byte("test-desktop-app")
	digestBytes := sha256.Sum256(content)
	digest := hex.EncodeToString(digestBytes[:])

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/public/updater/manifest":
			require.Equal(t, "desktop", r.URL.Query().Get("component"))
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schema_version": 1,
				"generated_at":   time.Now().UTC().Format(time.RFC3339),
				"expires_at":     time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339),
				"release": map[string]any{
					"version": "1.2.3",
					"channel": "stable",
				},
				"artifacts": map[string]any{
					"app": map[string]any{
						"id":           "app-artifact",
						"role":         "app",
						"os":           runtime.GOOS,
						"arch":         runtime.GOARCH,
						"package_type": "binary",
						"filename":     "desktop.bin",
						"download_url": server.URL + "/artifact",
						"size":         len(content),
						"sha256":       digest,
					},
				},
			})
		case "/artifact":
			_, _ = w.Write(content)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	logger := log.New(io.Discard, "", 0)
	var promptedVersion string
	var promptedSize int64
	current, err := EnsureCurrentApp(t.Context(), paths, server.URL, logger, func(version string, size int64) {
		promptedVersion = version
		promptedSize = size
	})
	require.NoError(t, err)
	require.Equal(t, "1.2.3", current.Version)
	require.Equal(t, "1.2.3", promptedVersion)
	require.Equal(t, int64(len(content)), promptedSize)
	require.FileExists(t, paths.AppPath("1.2.3"))

	installed, err := loadValidCurrent(paths)
	require.NoError(t, err)
	require.Equal(t, current.App, installed.App)

	server.Close()
	promptedVersion = ""
	cached, err := EnsureCurrentApp(t.Context(), paths, server.URL, logger, func(version string, size int64) {
		promptedVersion = version
	})
	require.NoError(t, err)
	require.Equal(t, "1.2.3", cached.Version)
	require.Empty(t, promptedVersion)
}

func TestAtomicWriteAndReadStateJSON(t *testing.T) {
	tmpDir := t.TempDir()
	paths, err := NewPaths(tmpDir)
	require.NoError(t, err)

	cur := CurrentInfo{
		SchemaVersion: 1,
		Version:       "1.1.0",
		App:           paths.LogicalAppName("1.1.0"),
		InstalledAt:   time.Now().UTC().Format(time.RFC3339),
	}

	require.NoError(t, atomicWriteJSON(paths.CurrentFile, &cur))

	var readCur CurrentInfo
	require.NoError(t, readStateJSON(paths.CurrentFile, &readCur))
	require.Equal(t, "1.1.0", readCur.Version)
	require.Equal(t, cur.App, readCur.App)
}

func TestDownloadAndVerifyArtifactSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	downloadsDir := filepath.Join(tmpDir, "downloads")
	require.NoError(t, os.MkdirAll(downloadsDir, 0700))

	content := []byte("binary-content-v1.1.1")
	h := sha256.New()
	h.Write(content)
	digest := hex.EncodeToString(h.Sum(nil))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer server.Close()

	art := launcheripc.ArtifactPayload{
		ID:          "art-1",
		Role:        "app",
		OS:          "linux",
		Arch:        "amd64",
		PackageType: "binary",
		Filename:    "beagle-signal-1.1.1.app",
		DownloadURL: server.URL,
		Size:        int64(len(content)),
		SHA256:      digest,
	}

	res, err := DownloadAndVerifyArtifact(t.Context(), downloadsDir, &art, nil)
	require.NoError(t, err)
	require.Equal(t, digest, res.SHA256)
	require.Equal(t, int64(len(content)), res.Size)
}

func TestCoordinatorStateTransitions(t *testing.T) {
	tmpDir := t.TempDir()
	paths, err := NewPaths(tmpDir)
	require.NoError(t, err)

	coord, err := NewCoordinator(paths)
	require.NoError(t, err)

	ctx := t.Context()

	// Connect
	connData, err := coord.HandleConnect(ctx, &launcheripc.ConnectRequest{
		PID:              200,
		Version:          "1.0.0",
		ProcessStartedAt: time.Now().UTC().Format(time.RFC3339),
	})
	require.NoError(t, err)
	require.Equal(t, "1.0.0", connData.ExpectedVersion)

	// State
	state, err := coord.GetState(ctx)
	require.NoError(t, err)
	require.Equal(t, "1.0.0", state.CurrentVersion)
}
