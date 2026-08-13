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
	"strings"
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
	currentContent, currentDigest := content, digest
	currentCommit := "1122334455667788990011223344556677889900"

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/public/updater/manifest":
			require.Equal(t, "desktop", r.URL.Query().Get("component"))
			now := time.Now().UTC()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schema_version": 1,
				"generated_at":   now.Format(time.RFC3339),
				"expires_at":     now.Add(10 * time.Minute).Format(time.RFC3339),
				"release": map[string]any{
					"version": "1.2.3", "commit_id": currentCommit,
					"published_at": now.Add(-time.Minute).Format(time.RFC3339), "channel": "stable",
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
						"size":         len(currentContent),
						"sha256":       currentDigest,
					},
				},
			})
		case "/artifact":
			_, _ = w.Write(currentContent)
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
	require.FileExists(t, filepath.Join(paths.VersionsDir, current.App))

	installed, err := loadValidCurrent(paths)
	require.NoError(t, err)
	require.Equal(t, current.App, installed.App)

	promptedVersion = ""
	cached, err := EnsureCurrentApp(t.Context(), paths, server.URL, logger, func(version string, size int64) {
		promptedVersion = version
	})
	require.NoError(t, err)
	require.Equal(t, "1.2.3", cached.Version)
	require.Empty(t, promptedVersion)
	require.Equal(t, digest, cached.Artifact.SHA256)

	currentContent = []byte("republished-desktop-app")
	nextDigestBytes := sha256.Sum256(currentContent)
	currentDigest = hex.EncodeToString(nextDigestBytes[:])
	unchanged, err := EnsureCurrentApp(t.Context(), paths, server.URL, logger, nil)
	require.NoError(t, err)
	require.Equal(t, "1.2.3", unchanged.Version)
	require.Equal(t, digest, unchanged.Artifact.SHA256)
	actualDigest, err := fileSHA256(filepath.Join(paths.VersionsDir, unchanged.App))
	require.NoError(t, err)
	require.Equal(t, digest, actualDigest)

	currentContent = []byte("runtime-republished-desktop-app")
	runtimeDigestBytes := sha256.Sum256(currentContent)
	currentDigest = hex.EncodeToString(runtimeDigestBytes[:])
	currentCommit = "abcdef0123456789abcdef0123456789abcdef01"
	coord, err := NewCoordinator(paths, server.URL)
	require.NoError(t, err)
	_, err = coord.HandleConnect(t.Context(), &launcheripc.ConnectRequest{PID: 200, Version: "1.2.3"})
	require.NoError(t, err)
	accepted, err := coord.HandleUpdateApply(t.Context(), &launcheripc.UpdateApplyRequest{
		SchemaVersion: 1, RequestID: "runtime-update", TargetVersion: "1.2.3",
		Artifact: launcheripc.ArtifactPayload{ID: "app-artifact", SHA256: currentDigest},
	})
	require.NoError(t, err)
	require.Equal(t, "runtime-update", accepted.OperationID)
	require.Equal(t, "waiting_for_exit", accepted.Phase)
	select {
	case <-coord.UpdateAccepted():
	case <-time.After(time.Second):
		t.Fatal("runtime update was not handed to Launcher")
	}
	require.NoError(t, coord.ExecuteAcceptedUpdate(t.Context()))
	runtimeCurrent := coord.Current()
	require.NotNil(t, runtimeCurrent)
	require.Equal(t, currentDigest, runtimeCurrent.Artifact.SHA256)
	runtimeActualDigest, err := fileSHA256(filepath.Join(paths.VersionsDir, runtimeCurrent.App))
	require.NoError(t, err)
	require.Equal(t, currentDigest, runtimeActualDigest)
}

func TestFetchPublicManifestDoesNotSendLocalBuildIdentity(t *testing.T) {
	commitID := "1122334455667788990011223344556677889900"
	digest := strings.Repeat("a", 64)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Empty(t, r.URL.Query().Get("current_version"))
		require.Empty(t, r.URL.Query().Get("current_commit_id"))
		now := time.Now().UTC()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"schema_version": 1, "generated_at": now.Format(time.RFC3339), "expires_at": now.Add(10 * time.Minute).Format(time.RFC3339),
			"release":   map[string]any{"version": "1.0.2", "commit_id": commitID, "channel": "stable", "published_at": now.Format(time.RFC3339)},
			"artifacts": map[string]any{"app": map[string]any{"id": "app", "role": "app", "os": runtime.GOOS, "arch": runtime.GOARCH, "package_type": "binary", "filename": "desktop.bin", "download_url": server.URL + "/artifact", "size": 1, "sha256": digest}},
		})
	}))
	defer server.Close()

	manifest, err := FetchPublicManifest(t.Context(), server.URL)
	require.NoError(t, err)
	require.True(t, EvaluateDesktopUpdate("1.0.2", "abcdef0", manifest).Available)
	require.False(t, EvaluateDesktopUpdate("1.0.2", "1122334", manifest).Available)
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

	coord, err := NewCoordinator(paths, "http://127.0.0.1")
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

}

func TestCoordinatorRequiresLocalAndServerHealth(t *testing.T) {
	paths, err := NewPaths(t.TempDir())
	require.NoError(t, err)
	current := &CurrentInfo{SchemaVersion: 1, Version: "1.1.0", App: paths.ArtifactAppName("1.1.0", strings.Repeat("b", 64)), Artifact: launcheripc.ArtifactPayload{SHA256: strings.Repeat("b", 64)}}
	previous := &CurrentInfo{SchemaVersion: 1, Version: "1.0.0", App: paths.ArtifactAppName("1.0.0", strings.Repeat("a", 64)), Artifact: launcheripc.ArtifactPayload{SHA256: strings.Repeat("a", 64)}}
	task := &UpdateTaskState{SchemaVersion: 1, OperationID: "op-health", TargetVersion: "1.1.0", TargetApp: current.App, Phase: "restarting", Artifact: current.Artifact}
	require.NoError(t, atomicWriteJSON(paths.CurrentFile, current))
	require.NoError(t, atomicWriteJSON(paths.PreviousFile, previous))
	require.NoError(t, atomicWriteJSON(paths.UpdateTaskFile, task))

	coord, err := NewCoordinator(paths, "http://127.0.0.1")
	require.NoError(t, err)
	coord.BeginAppRun(current, 301)
	_, err = coord.HandleConnect(t.Context(), &launcheripc.ConnectRequest{PID: 301, Version: "1.1.0"})
	require.NoError(t, err)
	require.NoError(t, coord.HandleAppReady(t.Context(), &launcheripc.AppReadyRequest{PID: 301, Version: "1.1.0"}))
	var persistedTask UpdateTaskState
	require.NoError(t, readStateJSON(paths.UpdateTaskFile, &persistedTask))
	require.Equal(t, "locally_healthy", persistedTask.Phase)
	require.NoError(t, coord.HandleServerHealthy(t.Context(), &launcheripc.ServerHealthyRequest{OperationID: "op-health", Version: "1.1.0"}))
	require.NoError(t, readStateJSON(paths.UpdateTaskFile, &persistedTask))
	require.Equal(t, "succeeded", persistedTask.Phase)
}

func TestCoordinatorAcceptsAppReadyBeforeHealthObservationStarts(t *testing.T) {
	paths, err := NewPaths(t.TempDir())
	require.NoError(t, err)
	current := &CurrentInfo{SchemaVersion: 1, Version: "1.1.0", App: paths.ArtifactAppName("1.1.0", strings.Repeat("b", 64)), Artifact: launcheripc.ArtifactPayload{SHA256: strings.Repeat("b", 64)}}
	previous := &CurrentInfo{SchemaVersion: 1, Version: "1.0.0", App: paths.ArtifactAppName("1.0.0", strings.Repeat("a", 64)), Artifact: launcheripc.ArtifactPayload{SHA256: strings.Repeat("a", 64)}}
	task := &UpdateTaskState{SchemaVersion: 1, OperationID: "op-early-ready", TargetVersion: "1.1.0", TargetApp: current.App, Phase: "restarting", Artifact: current.Artifact}
	require.NoError(t, atomicWriteJSON(paths.CurrentFile, current))
	require.NoError(t, atomicWriteJSON(paths.PreviousFile, previous))
	require.NoError(t, atomicWriteJSON(paths.UpdateTaskFile, task))

	coord, err := NewCoordinator(paths, "http://127.0.0.1")
	require.NoError(t, err)
	_, err = coord.HandleConnect(t.Context(), &launcheripc.ConnectRequest{PID: 302, Version: "1.1.0"})
	require.NoError(t, err)
	require.NoError(t, coord.HandleAppReady(t.Context(), &launcheripc.AppReadyRequest{PID: 302, Version: "1.1.0"}))

	coord.BeginAppRun(current, 302)
	var persistedTask UpdateTaskState
	require.NoError(t, readStateJSON(paths.UpdateTaskFile, &persistedTask))
	require.Equal(t, "locally_healthy", persistedTask.Phase)
	require.NoError(t, coord.HandleServerHealthy(t.Context(), &launcheripc.ServerHealthyRequest{OperationID: "op-early-ready", Version: "1.1.0"}))
	require.NoError(t, readStateJSON(paths.UpdateTaskFile, &persistedTask))
	require.Equal(t, "succeeded", persistedTask.Phase)
}

func TestCoordinatorRollsBackFailedUpdatedApp(t *testing.T) {
	paths, err := NewPaths(t.TempDir())
	require.NoError(t, err)
	current := &CurrentInfo{SchemaVersion: 1, Version: "1.1.0", App: paths.ArtifactAppName("1.1.0", strings.Repeat("b", 64)), Artifact: launcheripc.ArtifactPayload{SHA256: strings.Repeat("b", 64)}}
	previous := &CurrentInfo{SchemaVersion: 1, Version: "1.0.0", App: paths.ArtifactAppName("1.0.0", strings.Repeat("a", 64)), Artifact: launcheripc.ArtifactPayload{SHA256: strings.Repeat("a", 64)}}
	task := &UpdateTaskState{SchemaVersion: 1, OperationID: "op-rollback", TargetVersion: "1.1.0", TargetApp: current.App, Phase: "restarting", Artifact: current.Artifact}
	require.NoError(t, atomicWriteJSON(paths.CurrentFile, current))
	require.NoError(t, atomicWriteJSON(paths.PreviousFile, previous))
	require.NoError(t, atomicWriteJSON(paths.UpdateTaskFile, task))

	coord, err := NewCoordinator(paths, "http://127.0.0.1")
	require.NoError(t, err)
	require.True(t, coord.RollbackAfterFailure(&launcheripc.ErrorDetail{Code: launcheripc.ErrAppStartFailed, Message: "start failed"}))
	require.Equal(t, "1.0.0", coord.Current().Version)
	var persisted CurrentInfo
	require.NoError(t, readStateJSON(paths.CurrentFile, &persisted))
	require.Equal(t, "1.0.0", persisted.Version)
	var persistedTask UpdateTaskState
	require.NoError(t, readStateJSON(paths.UpdateTaskFile, &persistedTask))
	require.Equal(t, "rolled_back", persistedTask.Phase)
}
