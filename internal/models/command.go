package models

// VisitorCommand 是 Desktop-Web 发送给 Desktop-FRP 的命令
type VisitorCommand struct {
	Action       string // "connect" or "disconnect"
	InstanceID   int64
	InstanceName string
	SecretKey    string
	LocalPort    int
	Response     chan error // 用于同步等待操作结果
}

// VisitorStatus 是 Desktop-FRP 发送给 Desktop-Web 的状态更新
type VisitorStatus struct {
	InstanceID   int64
	InstanceName string
	Status       string // "connected", "disconnected", "error"
	LocalPort    int
	Error        string
}
