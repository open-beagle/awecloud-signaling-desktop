package client

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// TailscaleClient Tailscale API 客户端
type TailscaleClient struct {
	serverURL    string
	sessionToken string
	httpClient   *http.Client
}

// NewTailscaleClient 创建 TailscaleClient
func NewTailscaleClient(serverURL, sessionToken string) *TailscaleClient {
	return &TailscaleClient{
		serverURL:    serverURL,
		sessionToken: sessionToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// TailscaleAuthResponse Tailscale 认证响应
type TailscaleAuthResponse struct {
	Success    bool   `json:"success"`
	Message    string `json:"message,omitempty"`
	ControlURL string `json:"control_url,omitempty"`
	AuthKey    string `json:"auth_key,omitempty"`
	DerpURL    string `json:"derp_url,omitempty"`
}

// GetTailscaleAuth 获取 Tailscale 认证信息
func (c *TailscaleClient) GetTailscaleAuth() (*TailscaleAuthResponse, error) {
	url := fmt.Sprintf("%s/api/v1/client/tailscale/auth", c.serverURL)

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.sessionToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result TailscaleAuthResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response failed: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("get tailscale auth failed: %s", result.Message)
	}

	log.Printf("[TailscaleClient] Got auth info: control_url=%s", result.ControlURL)
	return &result, nil
}

// DisconnectTailscale 断开 Tailscale 连接
func (c *TailscaleClient) DisconnectTailscale() error {
	url := fmt.Sprintf("%s/api/v1/client/tailscale/disconnect", c.serverURL)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.sessionToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("[TailscaleClient] Disconnected from Tailscale")
	return nil
}

// ServiceV2Info Tailscale 模式的服务信息
type ServiceV2Info struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	AgentName   string `json:"agent_name"`
	TailscaleIP string `json:"tailscale_ip"`
	ListenPort  int    `json:"listen_port"`
	TargetAddr  string `json:"target_addr"`
	Status      string `json:"status"`
}

// GetServicesV2Response 获取服务列表响应
type GetServicesV2Response struct {
	Success  bool            `json:"success"`
	Message  string          `json:"message,omitempty"`
	Services []ServiceV2Info `json:"services"`
}

// GetServicesV2 获取服务列表（Tailscale 版本）
func (c *TailscaleClient) GetServicesV2() ([]ServiceV2Info, error) {
	url := fmt.Sprintf("%s/api/v1/client/services/v2", c.serverURL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.sessionToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result GetServicesV2Response
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response failed: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("get services failed: %s", result.Message)
	}

	log.Printf("[TailscaleClient] Got %d services", len(result.Services))
	return result.Services, nil
}
