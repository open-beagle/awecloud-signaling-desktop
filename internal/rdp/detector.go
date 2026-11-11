package rdp

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"
)

const DefaultRDPPort = 3389

// LocalRDPStatus 本机远程桌面（RDP）零握手探测快照
type LocalRDPStatus struct {
	Supported      bool   `json:"supported"`
	ServiceEnabled bool   `json:"service_enabled"`
	PortListening  bool   `json:"port_listening"`
	Ready          bool   `json:"ready"`
	LocalPort      int    `json:"local_port"`
	OS             string `json:"os"`
	Reason         string `json:"reason"`
}

// ProbeFunc 平台零握手本地 RDP 状态探测函数签名
type ProbeFunc func() LocalRDPStatus

// Detector 周期性执行本机零握手 RDP 状态探测并在状态变化时触发回调
type Detector struct {
	probe    ProbeFunc
	interval time.Duration

	mu       sync.RWMutex
	last     LocalRDPStatus
	hasLast  bool
	onChange func(LocalRDPStatus)
}

// NewDetector 创建本机 RDP 状态探测器
func NewDetector(interval time.Duration, onChange func(LocalRDPStatus)) *Detector {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	return &Detector{
		probe:    probePlatformLocalRDP,
		interval: interval,
		onChange: onChange,
	}
}

// ProbeNow 立即执行一次探测并在状态变化时触发回调
func (d *Detector) ProbeNow() LocalRDPStatus {
	status := d.probe()
	if status.LocalPort <= 0 {
		status.LocalPort = DefaultRDPPort
	}

	d.mu.Lock()
	changed := !d.hasLast ||
		d.last.Supported != status.Supported ||
		d.last.ServiceEnabled != status.ServiceEnabled ||
		d.last.PortListening != status.PortListening ||
		d.last.Ready != status.Ready ||
		d.last.LocalPort != status.LocalPort ||
		d.last.Reason != status.Reason
	d.last = status
	d.hasLast = true
	cb := d.onChange
	d.mu.Unlock()

	if changed && cb != nil {
		cb(status)
	}
	return status
}

// Current 返回最近一次探测结果（若尚未探测则立即探测一次）
func (d *Detector) Current() LocalRDPStatus {
	d.mu.RLock()
	if d.hasLast {
		st := d.last
		d.mu.RUnlock()
		return st
	}
	d.mu.RUnlock()
	return d.ProbeNow()
}

// Start 启动后台定时探测循环
func (d *Detector) Start(ctx context.Context) {
	d.ProbeNow()
	go func() {
		ticker := time.NewTicker(d.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				d.ProbeNow()
			}
		}
	}()
}

// parseWin32NetworkPort 解析 Win32 MIB_TCPROW / MIB_TCP6ROW 中的网络字节序端口号（低 16 位大端序）
func parseWin32NetworkPort(raw uint32) int {
	hi := (raw & 0xFF) << 8
	lo := (raw >> 8) & 0xFF
	return int(hi | lo)
}

// hasListeningPortInProcNetTCP 解析 Linux /proc/net/tcp 或 /proc/net/tcp6 文本，零套接字握手判断指定端口是否处于 LISTEN(0A) 状态
func hasListeningPortInProcNetTCP(content string, targetPort int) bool {
	if targetPort <= 0 || targetPort > 65535 {
		return false
	}
	lines := strings.Split(content, "\n")
	for idx, line := range lines {
		if idx == 0 {
			continue // 跳过表头
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		// st == "0A" 对应 TCP_LISTEN
		if !strings.EqualFold(fields[3], "0A") {
			continue
		}
		localParts := strings.Split(fields[1], ":")
		if len(localParts) != 2 {
			continue
		}
		portVal, err := strconv.ParseUint(localParts[1], 16, 16)
		if err != nil {
			continue
		}
		if int(portVal) == targetPort {
			return true
		}
	}
	return false
}
