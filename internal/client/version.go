package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"runtime"
	"time"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/version"
)

// VersionCheckRequest 版本检查请求
type VersionCheckRequest struct {
	ClientVersion string `json:"client_version"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
}

// VersionCheckResponse 版本检查响应
type VersionCheckResponse struct {
	Success      bool   `json:"success"`
	VersionValid bool   `json:"version_valid"`
	MinVersion   string `json:"min_version"`
	DownloadURL  string `json:"download_url"`
	Message      string `json:"message"`
}

// CheckVersion 检查客户端版本是否符合服务器要求
func CheckVersion(serverURL string) (*VersionCheckResponse, error) {
	// 构建请求
	req := &VersionCheckRequest{
		ClientVersion: version.GetVersion(),
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// 构建 API URL
	apiURL := serverURL + "/api/v1/client/version/check"
	log.Printf("[Version] Checking version: %s against server: %s", req.ClientVersion, apiURL)

	// 发送 HTTP 请求
	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var versionResp VersionCheckResponse
	if err := json.Unmarshal(body, &versionResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	log.Printf("[Version] Version check result: valid=%v, min_version=%s, message=%s",
		versionResp.VersionValid, versionResp.MinVersion, versionResp.Message)

	return &versionResp, nil
}
