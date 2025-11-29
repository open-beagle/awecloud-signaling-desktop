package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TunnelConfigClient 隧道配置客户端
type TunnelConfigClient struct {
	serverURL string
	token     string
	client    *http.Client
}

// NewTunnelConfigClient 创建隧道配置客户端
func NewTunnelConfigClient(serverURL, token string) *TunnelConfigClient {
	return &TunnelConfigClient{
		serverURL: serverURL,
		token:     token,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// TunnelConfigResponse 隧道配置响应
type TunnelConfigResponse struct {
	Success      bool   `json:"success"`
	TunnelServer string `json:"tunnel_server"`
	TunnelPort   int    `json:"tunnel_port"`
	TunnelToken  string `json:"tunnel_token"`
	Message      string `json:"message"`
}

// GetTunnelConfig 获取隧道配置
func (t *TunnelConfigClient) GetTunnelConfig() (*TunnelConfigResponse, error) {
	url := fmt.Sprintf("%s/api/v1/client/tunnel/config", t.serverURL)

	// 创建HTTP请求
	httpReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+t.token)

	// 发送请求
	resp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned error: %s (status: %d)", string(respBody), resp.StatusCode)
	}

	// 解析响应
	var result TunnelConfigResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("request failed: %s", result.Message)
	}

	return &result, nil
}
