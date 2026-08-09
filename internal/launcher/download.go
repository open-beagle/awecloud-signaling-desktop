package launcher

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/launcheripc"
)

type DownloadResult struct {
	PartPath string
	SHA256   string
	Size     int64
}

func DownloadAndVerifyArtifact(ctx context.Context, downloadsDir string, artifact *launcheripc.ArtifactPayload, pubKey ed25519.PublicKey, progressCallback func(int)) (*DownloadResult, error) {
	if artifact.Size <= 0 {
		return nil, errors.New("artifact size must be > 0")
	}

	partPath := filepath.Join(downloadsDir, fmt.Sprintf("%s.part", artifact.ID))
	_ = os.Remove(partPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifact.DownloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create download request failed: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download HTTP status %d", resp.StatusCode)
	}

	file, err := os.OpenFile(partPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("create part file failed: %w", err)
	}

	h := sha256.New()
	mw := io.MultiWriter(file, h)

	buffer := make([]byte, 32*1024)
	var downloaded int64

	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			downloaded += int64(n)
			if downloaded > artifact.Size {
				file.Close()
				_ = os.Remove(partPath)
				return nil, errors.New("download size exceeded expected size")
			}

			if _, wErr := mw.Write(buffer[:n]); wErr != nil {
				file.Close()
				_ = os.Remove(partPath)
				return nil, fmt.Errorf("write part file failed: %w", wErr)
			}

			if artifact.Size > 0 && progressCallback != nil {
				percent := int((downloaded * 100) / artifact.Size)
				progressCallback(percent)
			}
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			file.Close()
			_ = os.Remove(partPath)
			return nil, fmt.Errorf("read download stream failed: %w", err)
		}
	}

	file.Close()

	if downloaded != artifact.Size {
		_ = os.Remove(partPath)
		return nil, fmt.Errorf("downloaded size %d does not match expected size %d", downloaded, artifact.Size)
	}

	digest := hex.EncodeToString(h.Sum(nil))
	if digest != artifact.SHA256 {
		_ = os.Remove(partPath)
		return nil, fmt.Errorf("downloaded sha256 %s does not match expected %s", digest, artifact.SHA256)
	}

	if len(pubKey) > 0 && artifact.Signature != "" {
		sigBytes, err := base64.StdEncoding.DecodeString(artifact.Signature)
		if err != nil || !ed25519.Verify(pubKey, []byte(digest), sigBytes) {
			_ = os.Remove(partPath)
			return nil, errors.New("ed25519 signature verification failed")
		}
	}

	return &DownloadResult{
		PartPath: partPath,
		SHA256:   digest,
		Size:     downloaded,
	}, nil
}
