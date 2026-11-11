//go:build linux

package rdp

import (
	"fmt"
	"os"
)

func probePlatformLocalRDP() LocalRDPStatus {
	status := LocalRDPStatus{
		Supported: true,
		LocalPort: DefaultRDPPort,
		OS:        "linux",
	}

	listening := isLinuxTCPPortListening(DefaultRDPPort)
	status.ServiceEnabled = listening
	status.PortListening = listening
	status.Ready = listening
	if listening {
		status.Reason = fmt.Sprintf("检测到本地 xrdp/RDP 服务已在端口 %d 监听", DefaultRDPPort)
	} else {
		status.Reason = fmt.Sprintf("未检测到本地端口 %d 监听（如需被控可安装并启动 xrdp）", DefaultRDPPort)
	}
	return status
}

func isLinuxTCPPortListening(port int) bool {
	for _, procPath := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		data, err := os.ReadFile(procPath)
		if err != nil {
			continue
		}
		if hasListeningPortInProcNetTCP(string(data), port) {
			return true
		}
	}
	return false
}
