package client

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// HTTPFallback HTTP REST 回退客户端（gRPC 不可用时使用）
type HTTPFallback struct {
	serverURL  string
	httpClient *http.Client
	desktopID  uint64
	secret     string
}

// NewHTTPFallback 创建 HTTP 回退客户端
func NewHTTPFallback(serverURL string) *HTTPFallback {
	return &HTTPFallback{
		serverURL: serverURL,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		},
	}
}

// SetCredentials 设置认证凭证
func (h *HTTPFallback) SetCredentials(desktopID uint64, secret string) {
	h.desktopID = desktopID
	h.secret = secret
}

// Authenticate 认证（REST 版本）
func (h *HTTPFallback) Authenticate(desktopID uint64, secret, deviceFingerprint string, systemInfo *SystemInfoForREST) (*AuthResult, error) {
	reqBody := map[string]any{
		"desktop_id":         desktopID,
		"secret":             secret,
		"device_fingerprint": deviceFingerprint,
	}
	if systemInfo != nil {
		reqBody["system_info"] = systemInfo
	}

	var resp struct {
		Success   bool   `json:"success"`
		Message   string `json:"message"`
		AuthKey   string `json:"auth_key"`
		ServerURL string `json:"server_url"`
	}

	if err := h.post("/api/v1/desktop/authenticate", reqBody, &resp); err != nil {
		return nil, fmt.Errorf("REST authenticate failed: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("authentication failed: %s", resp.Message)
	}

	log.Printf("[HTTPFallback] Authentication successful via REST")

	return &AuthResult{
		Success:   true,
		DesktopID: desktopID,
		Secret:    secret,
		AuthKey:   resp.AuthKey,
		ServerURL: resp.ServerURL,
		Message:   resp.Message,
	}, nil
}

// SystemInfoForREST REST 请求用的系统信息
type SystemInfoForREST struct {
	OS        string `json:"os"`
	OSVersion string `json:"os_version"`
	Arch      string `json:"arch"`
	Hostname  string `json:"hostname"`
}

// CreateLoginSession 创建登录会话（REST 版本）
func (h *HTTPFallback) CreateLoginSession(usernameHint string) (*CreateLoginSessionResult, error) {
	reqBody := map[string]any{
		"username_hint": usernameHint,
	}

	var resp struct {
		Success   bool   `json:"success"`
		Message   string `json:"message"`
		SessionID string `json:"session_id"`
		LoginURL  string `json:"login_url"`
	}

	if err := h.post("/api/v1/desktop/create-login-session", reqBody, &resp); err != nil {
		return nil, fmt.Errorf("REST create login session failed: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("创建登录会话失败: %s", resp.Message)
	}

	return &CreateLoginSessionResult{
		Success:   true,
		Message:   resp.Message,
		SessionID: resp.SessionID,
		LoginURL:  resp.LoginURL,
	}, nil
}

// SendHeartbeat 发送心跳（REST 版本）
func (h *HTTPFallback) SendHeartbeat(tunnelIP string, tunnelConnected bool) error {
	reqBody := map[string]any{
		"tunnel_ip":        tunnelIP,
		"tunnel_connected": tunnelConnected,
	}

	var resp struct {
		Success bool `json:"success"`
	}

	if err := h.postWithAuth("/api/v1/desktop/heartbeat", reqBody, &resp); err != nil {
		return fmt.Errorf("REST heartbeat failed: %w", err)
	}

	return nil
}

// GetData 获取业务数据（REST 版本）
func (h *HTTPFallback) GetData() (*DataSnapshot, error) {
	var resp DataSnapshot

	if err := h.getWithAuth("/api/v1/desktop/data", &resp); err != nil {
		return nil, fmt.Errorf("REST get data failed: %w", err)
	}

	return &resp, nil
}

// DataSnapshot REST 数据快照
type DataSnapshot struct {
	Services           []any    `json:"services"`
	Hosts              []any    `json:"hosts"`
	Devices            []any    `json:"devices"`
	FavoriteServiceIDs []string `json:"favorite_service_ids"`
}

// post 发送 POST 请求
func (h *HTTPFallback) post(path string, body any, result any) error {
	return h.doRequest("POST", path, body, result, false)
}

// postWithAuth 发送带认证的 POST 请求
func (h *HTTPFallback) postWithAuth(path string, body any, result any) error {
	return h.doRequest("POST", path, body, result, true)
}

// getWithAuth 发送带认证的 GET 请求
func (h *HTTPFallback) getWithAuth(path string, result any) error {
	return h.doRequest("GET", path, nil, result, true)
}

// doRequest 执行 HTTP 请求
func (h *HTTPFallback) doRequest(method, path string, body any, result any, withAuth bool) error {
	url := h.serverURL + path

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	if withAuth && h.desktopID > 0 {
		req.Header.Set("X-Desktop-ID", fmt.Sprintf("%d", h.desktopID))
		req.Header.Set("X-Desktop-Secret", h.secret)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}

	return nil
}

// restHeartbeatLoop REST 模式心跳轮询
func (c *DesktopClient) restHeartbeatLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if !c.IsRESTMode() {
				return
			}

			c.tunnelMutex.RLock()
			ip := c.tunnelIP
			connected := c.tunnelConnected
			c.tunnelMutex.RUnlock()

			if err := c.httpFallback.SendHeartbeat(ip, connected); err != nil {
				log.Printf("[DesktopClient] REST heartbeat failed: %v", err)
			}
		}
	}
}

// restDataLoop REST 模式数据轮询
func (c *DesktopClient) restDataLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if !c.IsRESTMode() {
				return
			}

			// 拉取数据快照（目前只记录日志，后续可更新缓存）
			if _, err := c.httpFallback.GetData(); err != nil {
				log.Printf("[DesktopClient] REST data poll failed: %v", err)
			}
		}
	}
}
