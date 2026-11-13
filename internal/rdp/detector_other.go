//go:build !windows && !linux

package rdp

import "runtime"

func probePlatformLocalRDP() LocalRDPStatus {
	return LocalRDPStatus{
		Supported:      false,
		ServiceEnabled: false,
		PortListening:  false,
		Ready:          false,
		LocalPort:      DefaultRDPPort,
		OS:             runtime.GOOS,
		Reason:         "当前系统仅作为主控端发起远程桌面连接，不支持本机原生 RDP 3389 被控",
	}
}

func isPlatformTCPPortListening(_ int) (bool, bool) {
	return false, false
}
