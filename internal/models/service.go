package models

import "fmt"

// ServiceInfo 表示一个可访问的服务
type ServiceInfo struct {
	InstanceID    int64  `json:"instance_id"`
	InstanceName  string `json:"instance_name"`
	AgentName     string `json:"agent_name"`
	ServiceType   string `json:"service_type"`
	ServicePort   int    `json:"service_port"`   // Agent端的本地服务端口
	ServiceIP     string `json:"service_ip"`     // Agent端的本地服务IP
	PreferredPort int    `json:"preferred_port"` // 用户偏好的本地端口
	Description   string `json:"description"`
	SecretKey     string `json:"secret_key"`
	AccessType    string `json:"access_type"` // 'public', 'private', 'group'
	Status        string `json:"status"`      // 'online', 'offline'
	IsFavorite    bool   `json:"is_favorite"` // 是否收藏

	// Tailscale 模式新增字段
	AgentTailscaleIP string `json:"agent_tailscale_ip"` // Agent 的 Tailscale IP
	ListenPort       int    `json:"listen_port"`        // Agent 监听端口
	TargetAddr       string `json:"target_addr"`        // 内网目标地址（仅展示）
}

// ServiceInfoV2 Tailscale 模式的服务信息
type ServiceInfoV2 struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	AgentName   string `json:"agent_name"`
	TailscaleIP string `json:"tailscale_ip"`
	ListenPort  int    `json:"listen_port"`
	TargetAddr  string `json:"target_addr"`
	Status      string `json:"status"`
	Description string `json:"description"`
	IsFavorite  bool   `json:"is_favorite"`
}

// GetAccessAddr 获取访问地址（Tailscale 模式）
func (s *ServiceInfoV2) GetAccessAddr() string {
	if s.TailscaleIP != "" && s.ListenPort > 0 {
		return fmt.Sprintf("%s:%d", s.TailscaleIP, s.ListenPort)
	}
	return ""
}
