//go:build !darwin && !linux && !windows

// 其他平台 VIP 网络配置：空实现
package vip

import "log"

// NetworkConfig 其他平台网络配置管理器
type NetworkConfig struct{}

// NewNetworkConfig 创建网络配置管理器
func NewNetworkConfig() *NetworkConfig {
	return &NetworkConfig{}
}

// Setup 其他平台无需额外配置
func (n *NetworkConfig) Setup() error {
	log.Printf("[Network] 当前平台可能不支持 VIP 地址段，请手动配置")
	return nil
}

// Cleanup 其他平台无需清理
func (n *NetworkConfig) Cleanup() {}

// AddAlias 其他平台空实现
func (n *NetworkConfig) AddAlias(vip string) error {
	return nil
}
