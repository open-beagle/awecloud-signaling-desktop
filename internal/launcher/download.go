package launcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/launcheripc"
)

type DownloadResult struct {
	PartPath string
	SHA256   string
	Size     int64
}

func DownloadAndVerifyArtifact(ctx context.Context, downloadsDir string, artifact *launcheripc.ArtifactPayload, progressCallback func(int)) (*DownloadResult, error) {
	if artifact.Size <= 0 {
		return nil, errors.New("artifact size must be > 0")
	}

	if len(artifact.SHA256) != 64 {
		return nil, errors.New("artifact sha256 must contain 64 hexadecimal characters")
	}
	partPath := filepath.Join(downloadsDir, fmt.Sprintf("app-%s.part", strings.ToLower(artifact.SHA256)))
	_ = os.Remove(partPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifact.DownloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create download request failed: %w", err)
	}

	originURL, err := url.Parse(artifact.DownloadURL)
	if err != nil {
		return nil, fmt.Errorf("parse artifact URL failed: %w", err)
	}
	client := &http.Client{
		CheckRedirect: func(redirected *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many artifact redirects")
			}
			if !strings.EqualFold(redirected.URL.Scheme, originURL.Scheme) || !strings.EqualFold(redirected.URL.Host, originURL.Host) {
				return errors.New("cross-origin artifact redirect is forbidden")
			}
			return nil
		},
	}
	resp, err := client.Do(req)
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

	if err := file.Sync(); err != nil {
		file.Close()
		_ = os.Remove(partPath)
		return nil, fmt.Errorf("sync part file failed: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(partPath)
		return nil, fmt.Errorf("close part file failed: %w", err)
	}

	if downloaded != artifact.Size {
		_ = os.Remove(partPath)
		return nil, fmt.Errorf("downloaded size %d does not match expected size %d", downloaded, artifact.Size)
	}

	digest := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(digest, artifact.SHA256) {
		_ = os.Remove(partPath)
		return nil, fmt.Errorf("downloaded sha256 %s does not match expected %s", digest, artifact.SHA256)
	}

	return &DownloadResult{
		PartPath: partPath,
		SHA256:   digest,
		Size:     downloaded,
	}, nil
}
