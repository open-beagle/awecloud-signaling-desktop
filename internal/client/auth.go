package client

import (
	"context"
	"fmt"
	"log"
	"time"

	"awecloud-desktop/internal/config"
	"awecloud-desktop/internal/device"
	pb "awecloud-desktop/pkg/proto"
)

// AuthWithSecret 使用Client Secret登录并获取Device Token
func (c *DesktopClient) AuthWithSecret(clientID, clientSecret string, rememberMe bool) (*AuthResult, error) {
	// 获取设备指纹
	fingerprint, err := device.GetFingerprint()
	if err != nil {
		return nil, fmt.Errorf("failed to get device fingerprint: %w", err)
	}

	log.Printf("Authenticating with secret: client_id=%s, device=%s", clientID, fingerprint.Hash)

	// 调用认证API
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := &pb.AuthRequest{
		ClientId:     clientID,
		ClientSecret: clientSecret,
	}

	resp, err := c.grpcClient.Authenticate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("authentication failed: %s", resp.Message)
	}

	log.Printf("Authentication successful: session_token received")

	// 设置sessionToken到client
	c.sessionToken = resp.SessionToken
	c.clientID = clientID

	// 初始化审计日志客户端
	// HTTP API和gRPC在同一个端口（HTTP/2统一端口）
	webServerURL := fmt.Sprintf("http://%s", c.serverAddr)
	c.auditClient = NewAuditClient(webServerURL, resp.SessionToken)

	// 保存配置
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	cfg.ClientID = clientID
	cfg.RememberMe = rememberMe

	if rememberMe {
		cfg.ClientSecret = clientSecret
		cfg.DeviceToken = resp.SessionToken
		cfg.TokenExpiresAt = resp.ExpiresAt
	} else {
		cfg.ClientSecret = ""
		cfg.DeviceToken = ""
		cfg.TokenExpiresAt = 0
	}

	if err := cfg.Save(); err != nil {
		log.Printf("Warning: failed to save config: %v", err)
	}

	return &AuthResult{
		Success:      true,
		SessionToken: resp.SessionToken,
		ExpiresAt:    resp.ExpiresAt,
		TunnelToken:  resp.Token,
		TunnelServer: resp.Server,
		TunnelPort:   int(resp.Port),
		Message:      "Login successful",
	}, nil
}

// AuthWithToken 使用Device Token登录
func (c *DesktopClient) AuthWithToken(deviceToken string) (*AuthResult, error) {
	// 获取设备指纹
	fingerprint, err := device.GetFingerprint()
	if err != nil {
		return nil, fmt.Errorf("failed to get device fingerprint: %w", err)
	}

	log.Printf("Authenticating with device token: device=%s", fingerprint.Hash)

	// 调用Token认证API（使用GetServices来验证Token）
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := &pb.GetServicesRequest{
		SessionToken: deviceToken,
	}

	resp, err := c.grpcClient.GetServices(ctx, req)
	if err != nil {
		// Token无效或过期
		log.Printf("Token authentication failed: %v", err)
		return nil, fmt.Errorf("token authentication failed: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("token authentication failed")
	}

	log.Printf("Token authentication successful")

	// 设置sessionToken到client
	c.sessionToken = deviceToken

	// 初始化审计日志客户端
	// HTTP API和gRPC在同一个端口（HTTP/2统一端口）
	webServerURL := fmt.Sprintf("http://%s", c.serverAddr)
	c.auditClient = NewAuditClient(webServerURL, deviceToken)

	return &AuthResult{
		Success:      true,
		SessionToken: deviceToken,
		ExpiresAt:    0, // Token认证不返回新的过期时间
		Message:      "Login successful with device token",
	}, nil
}

// HandleTokenExpired 处理Token过期
func (c *DesktopClient) HandleTokenExpired() error {
	log.Printf("Token expired, clearing credentials")

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	cfg.ClearToken()
	return cfg.Save()
}

// AuthResult 认证结果
type AuthResult struct {
	Success      bool
	SessionToken string
	ExpiresAt    int64
	TunnelToken  string
	TunnelServer string
	TunnelPort   int
	Message      string
}
