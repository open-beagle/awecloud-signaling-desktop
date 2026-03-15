//go:build linux

// Linux VIP 网络配置：不需要额外操作
// Linux 的 127.0.0.0/8 整个地址段默认可用
package vip

import "log"

// NetworkConfig Linux 网络配置管理器
type NetworkConfig struct{}

// NewNetworkConfig 创建网络配置管理器
func NewNetworkConfig() *NetworkConfig {
	return &NetworkConfig{}
}

// Setup Linux 上 127.1.x.x 默认可用，无需额外配置
func (n *NetworkConfig) Setup() error {
	log.Printf("[Network] Linux 平台 127.1.x.x 默认可用")
	return nil
}

// Cleanup Linux 上无需清理
func (n *NetworkConfig) Cleanup() {}

// AddAlias Linux 上不需要逐个添加 alias
func (n *NetworkConfig) AddAlias(vip string) error {
	return nil
}
