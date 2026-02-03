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
	IsNewLogin bool   // 是否是首次登录
	DeviceName string // 设备名称（hostname）
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
	// 密码登录时 Server 总是返回 secret
	if resp.Secret != "" {
		config.GlobalConfig.DeviceToken = fmt.Sprintf("%d:%s", resp.DesktopId, resp.Secret)
		log.Printf("[DesktopClient] DeviceToken saved: desktop_id=%d", resp.DesktopId)
	} else {
		log.Printf("[WARN] [DesktopClient] No secret returned from server")
	}
	config.GlobalConfig.RememberMe = true
	if err := config.GlobalConfig.Save(); err != nil {
		log.Printf("[DesktopClient] Warning: failed to save config: %v", err)
	} else {
		log.Printf("[DesktopClient] Config saved successfully")
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
		DeviceName: fingerprint.Hostname,
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

// LogtoLoginResult Logto 登录结果
type LogtoLoginResult struct {
	Success    bool
	DesktopID  uint64
	Secret     string
	AuthKey    string
	ServerURL  string
	Message    string
	SessionID  string
	LoginURL   string
	UserInfo   *LogtoUserInfo
	DeviceName string
}

// LogtoUserInfo Logto 用户信息
type LogtoUserInfo struct {
	UserID      string
	Username    string
	Email       string
	DisplayName string
	Avatar      string
}

// LoginWithLogtoCallback Logto 登录回调
type LoginWithLogtoCallback func(result *LogtoLoginResult)

// LoginWithLogto 通过 Logto 登录（流式）
// 返回 login_url 后，调用方需要打开浏览器让用户完成登录
// 登录完成后通过 callback 通知结果
func (c *DesktopClient) LoginWithLogto(usernameHint string, callback LoginWithLogtoCallback) error {
	// 获取设备指纹
	fingerprint, err := device.GetFingerprint()
	if err != nil {
		return fmt.Errorf("failed to get device fingerprint: %w", err)
	}

	// 获取系统信息
	systemInfo, err := getSystemInfo()
	if err != nil {
		return fmt.Errorf("failed to get system info: %w", err)
	}

	log.Printf("[DesktopClient] LoginWithLogto: username_hint=%s, device=%s", usernameHint, fingerprint.Hash)

	// 调用 gRPC LoginWithLogto（流式）
	req := &pb.LogtoLoginRequest{
		DeviceFingerprint: fingerprint.Hash,
		DeviceName:        fingerprint.Hostname,
		UsernameHint:      usernameHint,
		SystemInfo:        systemInfo,
	}

	stream, err := c.grpcClient.LoginWithLogto(c.ctx, req)
	if err != nil {
		return fmt.Errorf("failed to start logto login: %w", err)
	}

	// 启动 goroutine 接收流式响应
	go func() {
		for {
			resp, err := stream.Recv()
			if err != nil {
				log.Printf("[DesktopClient] LoginWithLogto stream error: %v", err)
				callback(&LogtoLoginResult{
					Success: false,
					Message: fmt.Sprintf("登录失败: %v", err),
				})
				return
			}

			result := &LogtoLoginResult{
				SessionID:  resp.SessionId,
				LoginURL:   resp.LoginUrl,
				Message:    resp.Message,
				DeviceName: fingerprint.Hostname,
			}

			switch resp.Status {
			case pb.LogtoLoginStatus_LOGTO_LOGIN_STATUS_PENDING:
				// 返回 login_url，调用方需要打开浏览器
				log.Printf("[DesktopClient] LoginWithLogto pending: login_url=%s", resp.LoginUrl)
				callback(result)

			case pb.LogtoLoginStatus_LOGTO_LOGIN_STATUS_SUCCESS:
				// 登录成功
				log.Printf("[DesktopClient] LoginWithLogto success: desktop_id=%d", resp.DesktopId)
				result.Success = true
				result.DesktopID = resp.DesktopId
				result.Secret = resp.Secret
				result.AuthKey = resp.AuthKey
				result.ServerURL = resp.ServerUrl

				if resp.UserInfo != nil {
					result.UserInfo = &LogtoUserInfo{
						UserID:      resp.UserInfo.UserId,
						Username:    resp.UserInfo.Username,
						Email:       resp.UserInfo.Email,
						DisplayName: resp.UserInfo.Name,
						Avatar:      resp.UserInfo.Avatar,
					}
				}

				// 保存认证信息
				c.desktopID = resp.DesktopId
				c.secret = resp.Secret
				if result.UserInfo != nil {
					c.clientID = result.UserInfo.Username
				}

				// 保存到配置文件
				if result.UserInfo != nil {
					config.GlobalConfig.ClientID = result.UserInfo.Username
				}
				if resp.Secret != "" {
					config.GlobalConfig.DeviceToken = fmt.Sprintf("%d:%s", resp.DesktopId, resp.Secret)
				}
				config.GlobalConfig.RememberMe = true
				if err := config.GlobalConfig.Save(); err != nil {
					log.Printf("[DesktopClient] Warning: failed to save config: %v", err)
				}

				// 启动心跳
				if err := c.startHeartbeat("", false); err != nil {
					log.Printf("[DesktopClient] Warning: failed to start heartbeat: %v", err)
				}

				callback(result)
				return

			case pb.LogtoLoginStatus_LOGTO_LOGIN_STATUS_FAILED:
				log.Printf("[DesktopClient] LoginWithLogto failed: %s", resp.Message)
				result.Success = false
				callback(result)
				return

			case pb.LogtoLoginStatus_LOGTO_LOGIN_STATUS_EXPIRED:
				log.Printf("[DesktopClient] LoginWithLogto expired")
				result.Success = false
				result.Message = "登录超时，请重试"
				callback(result)
				return
			}
		}
	}()

	return nil
}
