package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DeviceClient 设备管理客户端
type DeviceClient struct {
	serverURL string
	token     string
	client    *http.Client
}

// NewDeviceClient 创建设备管理客户端
func NewDeviceClient(serverURL, token string) *DeviceClient {
	return &DeviceClient{
		serverURL: serverURL,
		token:     token,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// DeviceListResponse 设备列表响应
type DeviceListResponse struct {
	Success bool                 `json:"success"`
	Devices []DeviceInfoResponse `json:"devices"`
	Message string               `json:"message"`
}

// DeviceInfoResponse 设备信息响应
type DeviceInfoResponse struct {
	DeviceToken string     `json:"device_token"`
	DeviceInfo  DeviceInfo `json:"device_info"`
	CreatedAt   string     `json:"created_at"`
	LastUsedAt  string     `json:"last_used_at"`
	ExpiresAt   string     `json:"expires_at"`
	Revoked     bool       `json:"revoked"`
	IsCurrent   bool       `json:"is_current"`
}

// ListDevices 列出用户已登录的设备
func (d *DeviceClient) ListDevices() ([]DeviceInfoResponse, error) {
	url := fmt.Sprintf("%s/api/v1/client/auth/login/devices", d.serverURL)

	// 创建HTTP请求
	httpReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+d.token)

	// 发送请求
	resp, err := d.client.Do(httpReq)
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
	var result DeviceListResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("request failed: %s", result.Message)
	}

	return result.Devices, nil
}

// OfflineDevice 让设备下线（撤销Token）
func (d *DeviceClient) OfflineDevice(deviceToken string) error {
	url := fmt.Sprintf("%s/api/v1/client/auth/login/devices/%s/offline", d.serverURL, deviceToken)

	// 创建HTTP请求
	httpReq, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+d.token)

	// 发送请求
	resp, err := d.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned error: %s (status: %d)", string(respBody), resp.StatusCode)
	}

	// 解析响应
	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("offline failed: %s", result.Message)
	}

	return nil
}

// DeleteDevice 删除设备记录
func (d *DeviceClient) DeleteDevice(deviceToken string) error {
	url := fmt.Sprintf("%s/api/v1/client/auth/login/devices/%s", d.serverURL, deviceToken)

	// 创建HTTP请求
	httpReq, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+d.token)

	// 发送请求
	resp, err := d.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned error: %s (status: %d)", string(respBody), resp.StatusCode)
	}

	// 解析响应
	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("delete failed: %s", result.Message)
	}

	return nil
}
