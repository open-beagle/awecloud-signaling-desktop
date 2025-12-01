package client

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AuditClient 审计日志客户端
type AuditClient struct {
	serverURL string
	token     string
	client    *http.Client
}

// NewAuditClient 创建审计日志客户端
func NewAuditClient(serverURL, token string) *AuditClient {
	// 创建跳过TLS验证的HTTP客户端
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	return &AuditClient{
		serverURL: serverURL,
		token:     token,
		client: &http.Client{
			Timeout:   10 * time.Second,
			Transport: transport,
		},
	}
}

// DeviceInfo 设备信息（用于审计日志）
type DeviceInfo struct {
	OS        string `json:"os"`
	OSVersion string `json:"os_version"`
	Arch      string `json:"arch"`
	CPUModel  string `json:"cpu_model"`
	MachineID string `json:"machine_id"`
	Hostname  string `json:"hostname"`
}

// RecordConnectionRequest 记录连接审计日志请求
type RecordConnectionRequest struct {
	STCPInstanceID    int64      `json:"stcp_instance_id"`
	Action            string     `json:"action"` // connect, disconnect
	LocalPort         int        `json:"local_port"`
	DeviceFingerprint string     `json:"device_fingerprint"`
	DeviceInfo        DeviceInfo `json:"device_info"`
	Success           bool       `json:"success"`
	ErrorMessage      string     `json:"error_message"`
	ServerAddress     string     `json:"server_address"` // Desktop连接的Server地址
}

// RecordConnection 记录连接审计日志
func (a *AuditClient) RecordConnection(req *RecordConnectionRequest) error {
	url := fmt.Sprintf("%s/api/v1/client/audit/connection", a.serverURL)

	// 序列化请求
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// 创建HTTP请求
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.token)

	// 发送请求
	resp, err := a.client.Do(httpReq)
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
		return fmt.Errorf("record failed: %s", result.Message)
	}

	return nil
}
