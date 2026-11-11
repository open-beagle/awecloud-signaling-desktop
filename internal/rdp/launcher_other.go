//go:build !windows

package rdp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// LaunchRDPClient 在 macOS / Linux 下生成 0600 权限的临时 .rdp 配置文件并调用系统默认关联程序打开
func LaunchRDPClient(endpoint RDPEndpointInfo, appDir string) RDPConnectResult {
	target := endpoint.RecommendedTarget
	if target == "" {
		target = endpoint.LocalAddr
	}
	if appDir == "" {
		appDir = os.TempDir()
	}

	rdpDir := filepath.Join(appDir, "rdp")
	if err := os.MkdirAll(rdpDir, 0700); err != nil {
		return RDPConnectResult{
			Endpoint:     endpoint,
			Launched:     false,
			LaunchMethod: "manual",
			Message:      fmt.Sprintf("本地代理已就绪：%s（请使用 Windows App / Remmina / xfreerdp 手动连接）", target),
		}
	}

	filePath := filepath.Join(rdpDir, fmt.Sprintf("beagle-rdp-%d.rdp", endpoint.NodeID))
	rdpContent := fmt.Sprintf(
		"full address:s:%s\r\nprompt for credentials:i:1\r\nadministrative session:i:1\r\nredirectclipboard:i:1\r\n",
		target,
	)
	if err := os.WriteFile(filePath, []byte(rdpContent), 0600); err != nil {
		return RDPConnectResult{
			Endpoint:     endpoint,
			Launched:     false,
			LaunchMethod: "manual",
			Message:      fmt.Sprintf("本地代理已就绪：%s（生成 .rdp 文件失败：%v）", target, err),
		}
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("open", filePath)
	} else {
		cmd = exec.Command("xdg-open", filePath)
	}

	if err := cmd.Start(); err != nil {
		return RDPConnectResult{
			Endpoint:     endpoint,
			Launched:     false,
			LaunchMethod: "manual",
			RDPFilePath:  filePath,
			Message:      fmt.Sprintf("本地代理已就绪：%s（未检测到默认 RDP 客户端，可使用 Windows App 或 Remmina 连接）", target),
		}
	}

	return RDPConnectResult{
		Endpoint:     endpoint,
		Launched:     true,
		LaunchMethod: "rdp_file",
		RDPFilePath:  filePath,
		Message:      fmt.Sprintf("已通过系统远程桌面客户端打开 %s（如未安装 Windows App / Remmina，可手动连接该地址）", target),
	}
}
