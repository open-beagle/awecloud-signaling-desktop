package models

// ConnectionStatus 表示一个连接的状态
type ConnectionStatus struct {
	InstanceID   int64  `json:"instance_id"`
	InstanceName string `json:"instance_name"`
	Status       string `json:"status"` // "disconnected", "connecting", "connected", "error"
	LocalPort    int    `json:"local_port"`
	Error        string `json:"error,omitempty"`
}
