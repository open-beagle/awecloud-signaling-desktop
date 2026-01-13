package client

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/config"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/device"
	pb "github.com/open-beagle/awecloud-signaling-desktop/pkg/proto"
)

// AuthResult 认证结果
type AuthResult struct {
	Success    bool
	DesktopID  uint64
	Secret     string
	AuthKey    string
	ServerURL  string
	Message    string
	IsNewLogin bool // 是否是首次登录
}

// Login 首次登录（使用 Client 凭证）
func (c *DesktopClient) Login(clientName, clientSecret string) (*AuthResult, error) {
	// 获取设备指纹
	fingerprint, err := device.GetFingerprint()
	if err != nil {
		return nil, fmt.Errorf("failed to get device fingerprint: %w", err)
	}

	// 获取系统信息
	systemInfo, err := getSystemInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get system info: %w", err)
	}

	log.Printf("[DesktopClient] Login: client_name=%s, device=%s", clientName, fingerprint.Hash)

	// 调用 gRPC Login
	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()

	req := &pb.DesktopLoginRequest{
		ClientName:        clientName,
		ClientSecret:      clientSecret,
		DeviceName:        fingerprint.Hostname,
		DeviceFingerprint: fingerprint.Hash,
		SystemInfo:        systemInfo,
	}

	resp, err := c.grpcClient.Login(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("login failed: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("login failed: %s", resp.Message)
	}

	log.Printf("[DesktopClient] Login successful: desktop_id=%d", resp.DesktopId)

	// 保存认证信息
	c.desktopID = resp.DesktopId
	c.secret = resp.Secret
	c.clientID = clientName

	// 保存到配置文件
	config.GlobalConfig.ClientID = clientName
	config.GlobalConfig.DeviceToken = fmt.Sprintf("%d:%s", resp.DesktopId, resp.Secret)
	config.GlobalConfig.RememberMe = true
	if err := config.GlobalConfig.Save(); err != nil {
		log.Printf("[DesktopClient] Warning: failed to save config: %v", err)
	}

	// 启动心跳（初始状态：隧道未连接）
	if err := c.startHeartbeat("", false); err != nil {
		log.Printf("[DesktopClient] Warning: failed to start heartbeat: %v", err)
	}

	return &AuthResult{
		Success:    true,
		DesktopID:  resp.DesktopId,
		Secret:     resp.Secret,
		AuthKey:    resp.AuthKey,
		ServerURL:  resp.ServerUrl,
		Message:    resp.Message,
		IsNewLogin: true,
	}, nil
}

// Authenticate 认证（使用 Desktop 凭证）
func (c *DesktopClient) Authenticate(desktopID uint64, secret string) (*AuthResult, error) {
	// 获取设备指纹
	fingerprint, err := device.GetFingerprint()
	if err != nil {
		return nil, fmt.Errorf("failed to get device fingerprint: %w", err)
	}

	// 获取系统信息
	systemInfo, err := getSystemInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get system info: %w", err)
	}

	log.Printf("[DesktopClient] Authenticate: desktop_id=%d, device=%s", desktopID, fingerprint.Hash)

	// 调用 gRPC Authenticate
	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()

	req := &pb.DesktopAuthenticateRequest{
		DesktopId:         desktopID,
		Secret:            secret,
		DeviceFingerprint: fingerprint.Hash,
		SystemInfo:        systemInfo,
	}

	resp, err := c.grpcClient.Authenticate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("authentication failed: %s", resp.Message)
	}

	log.Printf("[DesktopClient] Authentication successful")

	// 保存认证信息
	c.desktopID = desktopID
	c.secret = secret

	// 启动心跳（初始状态：隧道未连接）
	if err := c.startHeartbeat("", false); err != nil {
		log.Printf("[DesktopClient] Warning: failed to start heartbeat: %v", err)
	}

	return &AuthResult{
		Success:    true,
		DesktopID:  desktopID,
		Secret:     secret,
		AuthKey:    resp.AuthKey,
		ServerURL:  resp.ServerUrl,
		Message:    resp.Message,
		IsNewLogin: false,
	}, nil
}

// TailscaleAuthInfo Tailscale 认证信息
type TailscaleAuthInfo struct {
	ControlURL string
	AuthKey    string
}

// GetTailscaleAuth 获取 Tailscale 认证信息（从认证结果中获取）
func (c *DesktopClient) GetTailscaleAuth(authResult *AuthResult) *TailscaleAuthInfo {
	return &TailscaleAuthInfo{
		ControlURL: authResult.ServerURL,
		AuthKey:    authResult.AuthKey,
	}
}
