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
		DeviceInfo: &pb.DeviceInfo{
			Os:       fingerprint.OS,
			Arch:     fingerprint.Arch,
			Hostname: fingerprint.Hostname,
		},
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
	// 使用原始的服务器URL（包含协议）
	c.auditClient = NewAuditClient(c.serverURL, resp.SessionToken)

	// 初始化设备管理客户端
	c.deviceClient = NewDeviceClient(c.serverURL, resp.SessionToken)

	// 初始化隧道配置客户端
	c.tunnelConfigClient = NewTunnelConfigClient(c.serverURL, resp.SessionToken)

	// 初始化服务收藏客户端
	c.favoriteClient = NewFavoriteClient(c.serverURL, resp.SessionToken)

	// 根据 rememberMe 决定是否保存配置
	config.GlobalConfig.ClientID = clientID
	config.GlobalConfig.RememberMe = rememberMe

	if rememberMe {
		// 保存配置到文件（包括 Token）
		config.GlobalConfig.ClientSecret = clientSecret
		config.GlobalConfig.DeviceToken = resp.DeviceToken
		config.GlobalConfig.TokenExpiresAt = resp.ExpiresAt
		log.Printf("Saving config with device token: %s", resp.DeviceToken[:16]+"...")
		if err := config.GlobalConfig.Save(); err != nil {
			log.Printf("Warning: failed to save config: %v", err)
		}
	} else {
		// 不保存配置，删除配置文件
		config.GlobalConfig.ClientSecret = ""
		config.GlobalConfig.DeviceToken = ""
		config.GlobalConfig.TokenExpiresAt = 0
		log.Printf("RememberMe=false, deleting config file")
		if err := config.Delete(); err != nil {
			log.Printf("Warning: failed to delete config: %v", err)
		}
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
func (c *DesktopClient) AuthWithToken(clientID, deviceToken string) (*AuthResult, error) {
	// 获取设备指纹
	fingerprint, err := device.GetFingerprint()
	if err != nil {
		return nil, fmt.Errorf("failed to get device fingerprint: %w", err)
	}

	log.Printf("Authenticating with device token: client_id=%s, device=%s", clientID, fingerprint.Hash)

	// 使用Device Token换取JWT Token
	jwtToken, err := c.exchangeDeviceTokenForJWT(clientID, deviceToken)
	if err != nil {
		log.Printf("Failed to exchange device token for JWT: %v", err)
		// 清除无效的Token
		if config.GlobalConfig != nil {
			config.GlobalConfig.ClearToken()
			config.GlobalConfig.Save()
		}
		// 返回更友好的错误信息
		return nil, fmt.Errorf("登录凭据已过期，请重新输入密码")
	}

	log.Printf("Token authentication successful, got JWT token")

	// 设置sessionToken到client（使用JWT）
	c.sessionToken = jwtToken

	// 初始化审计日志客户端
	// 使用原始的服务器URL（包含协议）和JWT token
	c.auditClient = NewAuditClient(c.serverURL, jwtToken)

	// 初始化设备管理客户端
	c.deviceClient = NewDeviceClient(c.serverURL, jwtToken)

	// 初始化隧道配置客户端
	c.tunnelConfigClient = NewTunnelConfigClient(c.serverURL, jwtToken)

	// 初始化服务收藏客户端
	c.favoriteClient = NewFavoriteClient(c.serverURL, jwtToken)

	// 不再从配置文件读取隧道配置
	// 隧道配置将在连接服务时从Server动态获取
	log.Printf("Token authentication successful, tunnel config will be fetched when connecting to services")

	result := &AuthResult{
		Success:      true,
		SessionToken: jwtToken,
		ExpiresAt:    0, // Token认证不返回新的过期时间
		Message:      "Login successful with device token",
	}

	return result, nil
}

// HandleTokenExpired 处理Token过期
func (c *DesktopClient) HandleTokenExpired() error {
	log.Printf("Token expired, clearing credentials")

	if config.GlobalConfig == nil {
		return fmt.Errorf("global config not initialized")
	}

	config.GlobalConfig.ClearToken()
	return config.GlobalConfig.Save()
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

// exchangeDeviceTokenForJWT 使用Device Token换取JWT Token
func (c *DesktopClient) exchangeDeviceTokenForJWT(clientID, deviceToken string) (string, error) {
	// 调用Server API: POST /api/v1/client/auth/login/token
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := &pb.LoginWithTokenRequest{
		ClientId:    clientID,
		DeviceToken: deviceToken,
	}

	resp, err := c.grpcClient.LoginWithToken(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to call LoginWithToken API: %w", err)
	}

	if !resp.Success {
		return "", fmt.Errorf("login with token failed: %s", resp.Message)
	}

	if resp.JwtToken == "" {
		return "", fmt.Errorf("server did not return JWT token")
	}

	log.Printf("Successfully exchanged device token for JWT")
	return resp.JwtToken, nil
}
