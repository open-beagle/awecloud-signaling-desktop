package launcher

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

	name := paths.LogicalAppName("1.2.3")
	require.Contains(t, name, "beagle-signal-1.2.3.app")
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

	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	content := []byte("binary-content-v1.1.1")
	h := sha256.New()
	h.Write(content)
	digest := hex.EncodeToString(h.Sum(nil))

	sigBytes := ed25519.Sign(privKey, []byte(digest))
	sigBase64 := base64.StdEncoding.EncodeToString(sigBytes)

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
		Signature:   sigBase64,
		KeyID:       "key-1",
	}

	res, err := DownloadAndVerifyArtifact(t.Context(), downloadsDir, &art, pubKey, nil)
	require.NoError(t, err)
	require.Equal(t, digest, res.SHA256)
	require.Equal(t, int64(len(content)), res.Size)
}

func TestCoordinatorStateTransitions(t *testing.T) {
	tmpDir := t.TempDir()
	paths, err := NewPaths(tmpDir)
	require.NoError(t, err)

	pubKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	coord, err := NewCoordinator(paths, pubKey)
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
