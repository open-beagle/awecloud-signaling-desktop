package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// buildAddress 默认服务器地址（通过 ldflags 注入）
var buildAddress = "localhost:8080"

// GlobalConfig 全局配置对象（由 App 初始化时设置）
var GlobalConfig *Config

// Config 是 Desktop 应用的配置（内存中使用）
type Config struct {
	ServerAddress   string        `json:"server_address"`   // Server gRPC 地址，例如 "localhost:8081"
	ClientID        string        `json:"client_id"`        // Client ID（用户名/邮箱）
	ClientSecret    string        `json:"client_secret"`    // Client Secret（加密存储）
	DeviceToken     string        `json:"device_token"`     // Device Token（用于自动登录）
	RememberMe      bool          `json:"remember_me"`      // 是否记住登录
	TokenExpiresAt  int64         `json:"token_expires_at"` // Token 过期时间（Unix 时间戳）
	TunnelToken     string        `json:"tunnel_token"`     // 隧道认证 Token
	TunnelServer    string        `json:"tunnel_server"`    // 隧道服务器地址
	TunnelPort      int           `json:"tunnel_port"`      // 隧道服务器端口
	PortPreferences map[int64]int `json:"port_preferences"` // 服务 ID -> 本地端口映射
}

// LocalConfig 是保存到本地文件的配置（精简版）
type LocalConfig struct {
	Server string `json:"server"`          // Server 地址
	Client string `json:"client"`          // Client ID（用户名/邮箱）
	Token  string `json:"token,omitempty"` // Device Token（用于自动登录）
}

// GetConfigPath 返回配置文件的路径
func GetConfigPath() (string, error) {
	// 获取用户配置目录
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	// 创建应用配置目录
	appConfigDir := filepath.Join(configDir, "awecloud-signaling")
	if err := os.MkdirAll(appConfigDir, 0755); err != nil {
		return "", err
	}

	return filepath.Join(appConfigDir, "desktop.json"), nil
}

// Load 从文件加载配置
func Load() (*Config, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	// 如果文件不存在，返回默认配置
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return &Config{
			ServerAddress:   buildAddress,
			RememberMe:      true,
			PortPreferences: make(map[int64]int),
		}, nil
	}

	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	// 解析为 LocalConfig
	var localConfig LocalConfig
	if err := json.Unmarshal(data, &localConfig); err != nil {
		// 解析失败，返回默认配置
		return &Config{
			ServerAddress:   buildAddress,
			RememberMe:      true,
			PortPreferences: make(map[int64]int),
		}, nil
	}

	// 转换为 Config
	config := &Config{
		ServerAddress:   localConfig.Server,
		ClientID:        localConfig.Client,
		DeviceToken:     localConfig.Token,
		RememberMe:      localConfig.Token != "", // 有 token 就是记住登录
		PortPreferences: make(map[int64]int),
	}

	// 如果没有服务器地址，使用默认值
	if config.ServerAddress == "" {
		config.ServerAddress = buildAddress
	}

	return config, nil
}

// Save 保存配置到文件
func (c *Config) Save() error {
	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}

	// 转换为 LocalConfig（只保存 3 个字段）
	localConfig := LocalConfig{
		Server: c.ServerAddress,
		Client: c.ClientID,
		Token:  c.DeviceToken,
	}

	data, err := json.MarshalIndent(localConfig, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0600)
}

// ClearToken 清除所有认证信息
func (c *Config) ClearToken() {
	c.ClientSecret = ""
	c.DeviceToken = ""
	c.TokenExpiresAt = 0
	c.TunnelToken = ""
	c.TunnelServer = ""
	c.TunnelPort = 0
}

// ShouldAutoFill 判断是否应该自动填充登录信息
func (c *Config) ShouldAutoFill() bool {
	return c.RememberMe && c.ServerAddress != "" && c.ClientID != ""
}

// HasValidToken 判断是否有有效的Device Token
func (c *Config) HasValidToken() bool {
	return c.DeviceToken != ""
}

// IsTokenExpired 判断Token是否过期
func (c *Config) IsTokenExpired() bool {
	if c.TokenExpiresAt == 0 {
		return true
	}
	// 提前5分钟判断为过期，避免边界情况
	return c.TokenExpiresAt < (getCurrentTimestamp() + 300)
}

// getCurrentTimestamp 获取当前Unix时间戳
func getCurrentTimestamp() int64 {
	return time.Now().Unix()
}

// Delete 删除配置文件
func Delete() error {
	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}

	// 删除配置文件（如果不存在也不报错）
	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}
