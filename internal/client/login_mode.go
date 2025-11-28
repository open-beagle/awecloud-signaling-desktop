package client

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/config"
)

// LoginMode 登录模式
type LoginMode int

const (
	// LoginModeOffline 离线模式 - 显示服务器地址和用户名，提示离线
	LoginModeOffline LoginMode = iota
	// LoginModeFull 完整登录模式 - 显示完整登录表单
	LoginModeFull
)

// LoginModeInfo 登录模式信息
type LoginModeInfo struct {
	Mode          LoginMode
	ServerAddress string
	ClientID      string
	CanAutoFill   bool
	HasToken      bool
	IsOnline      bool
}

// DetermineLoginMode 判断登录模式
func DetermineLoginMode(cfg *config.Config) (*LoginModeInfo, error) {
	info := &LoginModeInfo{
		ServerAddress: cfg.ServerAddress,
		ClientID:      cfg.ClientID,
		CanAutoFill:   cfg.ShouldAutoFill(),
		HasToken:      cfg.HasValidToken(),
	}

	// 检查Server连接状态
	info.IsOnline = CanConnectToServer(cfg.ServerAddress)

	// 判断登录模式
	if info.HasToken && !info.IsOnline {
		// 模式1：有Token但Server离线 - 显示离线状态
		info.Mode = LoginModeOffline
		log.Printf("[LoginMode] Offline mode: has token but server is offline")
	} else {
		// 模式2：完整登录模式
		info.Mode = LoginModeFull
		log.Printf("[LoginMode] Full login mode: online=%v, has_token=%v", info.IsOnline, info.HasToken)
	}

	return info, nil
}

// CanConnectToServer 检查是否可以连接到Server
func CanConnectToServer(serverAddr string) bool {
	if serverAddr == "" {
		return false
	}

	// 尝试建立gRPC连接
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		ctx,
		serverAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		log.Printf("[LoginMode] Cannot connect to server %s: %v", serverAddr, err)
		return false
	}
	defer conn.Close()

	// 检查连接状态
	state := conn.GetState()
	log.Printf("[LoginMode] Server %s connection state: %v", serverAddr, state)

	return true
}

// ShouldAutoLogin 判断是否应该自动登录
func ShouldAutoLogin(cfg *config.Config) bool {
	// 有有效Token且未过期
	if !cfg.HasValidToken() {
		return false
	}

	if cfg.IsTokenExpired() {
		return false
	}

	// Server在线
	if !CanConnectToServer(cfg.ServerAddress) {
		return false
	}

	return true
}

// GetLoginHint 获取登录提示信息
func GetLoginHint(info *LoginModeInfo) string {
	if info.Mode == LoginModeOffline {
		return "Server is offline. You can view cached services but cannot connect to new services."
	}

	if info.CanAutoFill {
		return "Welcome back! Please enter your password to continue."
	}

	return "Please enter your credentials to login."
}
