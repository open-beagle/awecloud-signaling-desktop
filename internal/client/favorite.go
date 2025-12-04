package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// FavoriteClient 服务收藏客户端
type FavoriteClient struct {
	serverURL string
	token     string
	client    *http.Client
}

// NewFavoriteClient 创建服务收藏客户端
func NewFavoriteClient(serverURL, token string) *FavoriteClient {
	return &FavoriteClient{
		serverURL: serverURL,
		token:     token,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// FavoriteInfo 收藏信息
type FavoriteInfo struct {
	STCPInstanceID int64 `json:"stcp_instance_id"`
	LocalPort      int   `json:"local_port"`
}

// GetFavoritesResponse 获取收藏列表响应
type GetFavoritesResponse struct {
	Success   bool           `json:"success"`
	Favorites []FavoriteInfo `json:"favorites"`
	Message   string         `json:"message"`
}

// ToggleFavoriteRequest 切换收藏请求
type ToggleFavoriteRequest struct {
	STCPInstanceID int64 `json:"stcp_instance_id"`
	LocalPort      int   `json:"local_port,omitempty"`
}

// ToggleFavoriteResponse 切换收藏响应
type ToggleFavoriteResponse struct {
	Success    bool   `json:"success"`
	IsFavorite bool   `json:"is_favorite"`
	Message    string `json:"message"`
}

// GetFavorites 获取用户的服务收藏列表（包含端口信息）
func (f *FavoriteClient) GetFavorites() ([]FavoriteInfo, error) {
	url := fmt.Sprintf("%s/api/v1/client/favorites", f.serverURL)

	// 创建HTTP请求
	httpReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+f.token)

	// 发送请求
	resp, err := f.client.Do(httpReq)
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
	var result GetFavoritesResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("request failed: %s", result.Message)
	}

	return result.Favorites, nil
}

// ToggleFavorite 切换服务收藏状态（可选指定端口）
func (f *FavoriteClient) ToggleFavorite(instanceID int64, localPort int) (bool, error) {
	url := fmt.Sprintf("%s/api/v1/client/favorites/toggle", f.serverURL)

	// 构建请求体
	reqBody := ToggleFavoriteRequest{
		STCPInstanceID: instanceID,
		LocalPort:      localPort,
	}
	reqData, err := json.Marshal(reqBody)
	if err != nil {
		return false, fmt.Errorf("failed to marshal request: %w", err)
	}

	// 创建HTTP请求
	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(reqData))
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+f.token)
	httpReq.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := f.client.Do(httpReq)
	if err != nil {
		return false, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read response: %w", err)
	}

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("server returned error: %s (status: %d)", string(respBody), resp.StatusCode)
	}

	// 解析响应
	var result ToggleFavoriteResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return false, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success {
		return false, fmt.Errorf("toggle favorite failed: %s", result.Message)
	}

	return result.IsFavorite, nil
}

// UpdateFavoritePortRequest 更新收藏端口请求
type UpdateFavoritePortRequest struct {
	STCPInstanceID int64 `json:"stcp_instance_id"`
	LocalPort      int   `json:"local_port"`
}

// UpdateFavoritePort 更新收藏服务的端口
func (f *FavoriteClient) UpdateFavoritePort(instanceID int64, localPort int) error {
	url := fmt.Sprintf("%s/api/v1/client/favorites/port", f.serverURL)

	// 构建请求体
	reqBody := UpdateFavoritePortRequest{
		STCPInstanceID: instanceID,
		LocalPort:      localPort,
	}
	reqData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// 创建HTTP请求
	httpReq, err := http.NewRequest("PUT", url, bytes.NewBuffer(reqData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+f.token)
	httpReq.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := f.client.Do(httpReq)
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
		return fmt.Errorf("update port failed: %s", result.Message)
	}

	return nil
}
