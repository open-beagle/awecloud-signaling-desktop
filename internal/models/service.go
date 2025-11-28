package models

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
}
