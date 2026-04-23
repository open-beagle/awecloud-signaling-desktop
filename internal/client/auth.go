package client

import (
	"context"
	"fmt"
	"log"
	"time"

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
	IsNewLogin bool   // 是否是首次登录
	DeviceName string // 设备名称（hostname）
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

	// 先尝试 gRPC
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
		log.Printf("[DesktopClient] gRPC Authenticate failed: %v, trying REST fallback...", err)

		// gRPC 失败，尝试 REST 回退
		restResult, restErr := c.httpFallback.Authenticate(desktopID, secret, fingerprint.Hash, &SystemInfoForREST{
			OS:       fingerprint.OS,
			Arch:     fingerprint.Arch,
			Hostname: fingerprint.Hostname,
		})
		if restErr != nil {
			// REST 也失败，返回原始 gRPC 错误
			return nil, fmt.Errorf("authentication failed: %w", err)
		}

		// REST 成功，切换到 REST 模式
		c.switchToREST()
		log.Printf("[DesktopClient] Authentication successful via REST fallback")

		// 保存认证信息
		c.desktopID = desktopID
		c.secret = secret
		c.httpFallback.SetCredentials(desktopID, secret)

		// REST 模式下启动轮询心跳
		go c.restHeartbeatLoop()
		// REST 模式下启动轮询数据
		go c.restDataLoop()

		return restResult, nil
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

	// 启动数据流（接收 Server 推送的业务数据）
	if err := c.startDataStream(); err != nil {
		log.Printf("[DesktopClient] Warning: failed to start data stream: %v", err)
	}

	return &AuthResult{
		Success:    true,
		DesktopID:  desktopID,
		Secret:     secret,
		AuthKey:    resp.AuthKey,
		ServerURL:  resp.ServerUrl,
		Message:    resp.Message,
		IsNewLogin: false,
		DeviceName: fingerprint.Hostname,
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

// CreateLoginSessionResult 创建登录会话结果
type CreateLoginSessionResult struct {
	Success   bool
	Message   string
	SessionID string
	LoginURL  string // 相对路径 /auth/desktop/{session_id}
}

// CreateLoginSession 通过 gRPC 创建登录会话
func (c *DesktopClient) CreateLoginSession(usernameHint string) (*CreateLoginSessionResult, error) {
	// 获取设备指纹
	fingerprint, err := device.GetFingerprint()
	if err != nil {
		return nil, fmt.Errorf("获取设备指纹失败: %w", err)
	}

	log.Printf("[Client] CreateLoginSession: usernameHint=%s", usernameHint)

	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()

	req := &pb.CreateLoginSessionRequest{
		UsernameHint:      usernameHint,
		DeviceFingerprint: fingerprint.Hash,
		DeviceName:        fingerprint.Hostname,
	}

	resp, err := c.grpcClient.CreateLoginSession(ctx, req)
	if err != nil {
		log.Printf("[Client] gRPC CreateLoginSession failed: %v, trying REST fallback...", err)

		// gRPC 失败，尝试 REST 回退
		restResult, restErr := c.httpFallback.CreateLoginSession(usernameHint)
		if restErr != nil {
			return nil, fmt.Errorf("创建登录会话失败: %w", err)
		}

		c.switchToREST()
		return restResult, nil
	}

	if !resp.Success {
		return nil, fmt.Errorf("创建登录会话失败: %s", resp.Message)
	}

	log.Printf("[Client] CreateLoginSession 成功: sessionID=%s, loginURL=%s", resp.SessionId, resp.LoginUrl)

	return &CreateLoginSessionResult{
		Success:   true,
		Message:   resp.Message,
		SessionID: resp.SessionId,
		LoginURL:  resp.LoginUrl,
	}, nil
}
