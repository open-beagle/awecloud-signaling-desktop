package models

// ServiceInfo 表示一个可访问的服务
type ServiceInfo struct {
	InstanceID   int64  `json:"instance_id"`
	InstanceName string `json:"instance_name"`
	AgentName    string `json:"agent_name"`
	ServiceType  string `json:"service_type"`
	ServicePort  int    `json:"service_port"`
	Description  string `json:"description"`
	SecretKey    string `json:"secret_key"`
}
