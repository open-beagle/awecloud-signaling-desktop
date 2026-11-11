//go:build windows

package rdp

import (
	"fmt"
	"os/exec"
)

// LaunchRDPClient 在 Windows 本机直接拉起原生 mstsc.exe 连接指定本地代理地址
func LaunchRDPClient(endpoint RDPEndpointInfo, _ string) RDPConnectResult {
	target := endpoint.RecommendedTarget
	if target == "" {
		target = endpoint.LocalAddr
	}

	cmd := exec.Command("mstsc.exe", fmt.Sprintf("/v:%s", target))
	if err := cmd.Start(); err != nil {
		return RDPConnectResult{
			Endpoint:     endpoint,
			Launched:     false,
			LaunchMethod: "manual",
			Message:      fmt.Sprintf("自动启动 mstsc.exe 失败（%v），请使用远程桌面连接 %s", err, target),
		}
	}

	return RDPConnectResult{
		Endpoint:     endpoint,
		Launched:     true,
		LaunchMethod: "mstsc",
		Message:      fmt.Sprintf("已启动 Windows 远程桌面连接 %s", target),
	}
}
