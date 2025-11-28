package client

import (
	"context"
	"crypto/tls"
	"log"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
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

	// 移除协议前缀以获取实际地址
	addr := serverAddr
	addr = strings.TrimPrefix(addr, "https://")
	addr = strings.TrimPrefix(addr, "http://")
	addr = strings.TrimPrefix(addr, "wss://")
	addr = strings.TrimPrefix(addr, "ws://")

	// 移除路径部分
	if idx := strings.Index(addr, "/"); idx != -1 {
		addr = addr[:idx]
	}

	// 根据原始地址判断是否使用 TLS
	var opts []grpc.DialOption
	if strings.HasPrefix(serverAddr, "https://") || strings.HasPrefix(serverAddr, "wss://") {
		// 使用 TLS，跳过证书验证
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// 尝试建立gRPC连接
	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		log.Printf("[LoginMode] Cannot create client for server %s: %v", addr, err)
		return false
	}
	defer conn.Close()

	// 尝试连接并检查状态
	conn.Connect()

	// 创建超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 等待连接状态变化
	for {
		state := conn.GetState()
		log.Printf("[LoginMode] Server %s connection state: %v", addr, state)

		if state == connectivity.Ready {
			return true
		}

		if state == connectivity.TransientFailure || state == connectivity.Shutdown {
			return false
		}

		// 等待状态变化或超时
		if !conn.WaitForStateChange(ctx, state) {
			// 超时或上下文取消
			log.Printf("[LoginMode] Connection timeout for server %s", addr)
			return false
		}
	}
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
