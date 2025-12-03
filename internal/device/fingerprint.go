package device

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/denisbrodbeck/machineid"
)

// Fingerprint 设备指纹信息
type Fingerprint struct {
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Hostname  string `json:"hostname"`
	MachineID string `json:"machine_id"`
	Hash      string `json:"hash"`
}

// GetFingerprint 获取设备指纹
func GetFingerprint() (*Fingerprint, error) {
	// 获取操作系统（使用友好的显示名称）
	osName := GetOSInfo()

	// 获取架构（使用友好的显示名称）
	arch := GetArchInfo()

	// 获取主机名
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// 获取机器ID
	machineID, err := machineid.ID()
	if err != nil {
		return nil, fmt.Errorf("failed to get machine ID: %w", err)
	}

	// 生成指纹哈希（使用原始值）
	hash := generateHash(runtime.GOOS, runtime.GOARCH, hostname, machineID)

	return &Fingerprint{
		OS:        osName,
		Arch:      arch,
		Hostname:  hostname,
		MachineID: machineID,
		Hash:      hash,
	}, nil
}

// generateHash 生成设备指纹哈希
func generateHash(os, arch, hostname, machineID string) string {
	// 组合所有信息
	data := strings.Join([]string{os, arch, hostname, machineID}, "|")

	// 计算SHA256哈希
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// GetOSInfo 获取操作系统信息（用于显示）
func GetOSInfo() string {
	switch runtime.GOOS {
	case "windows":
		// 尝试获取Windows版本
		version := getWindowsVersion()
		if version != "" {
			return version
		}
		return "Windows"
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	default:
		return runtime.GOOS
	}
}

// getWindowsVersion 获取Windows版本号
func getWindowsVersion() string {
	// 使用 wmic 命令获取 Windows 版本和 Build 号
	// Windows 11 的 Build 号 >= 22000

	cmd := exec.Command("wmic", "os", "get", "Caption,BuildNumber", "/value")
	output, err := cmd.Output()
	if err != nil {
		// 如果命令失败，返回通用的 Windows
		return "Windows"
	}

	outputStr := string(output)
	lines := strings.Split(outputStr, "\n")

	var caption string
	var buildNumber int

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "BuildNumber=") {
			buildStr := strings.TrimPrefix(line, "BuildNumber=")
			buildNumber, _ = strconv.Atoi(strings.TrimSpace(buildStr))
		}
		if strings.HasPrefix(line, "Caption=") {
			caption = strings.TrimPrefix(line, "Caption=")
			caption = strings.TrimSpace(caption)
		}
	}

	// Windows 11 的判断：Build 号 >= 22000
	if buildNumber >= 22000 {
		return "Windows 11"
	}

	// Windows 10 的判断：Build 号 >= 10240
	if buildNumber >= 10240 {
		return "Windows 10"
	}

	// 如果能从 Caption 中提取版本信息
	if caption != "" {
		// Caption 通常是 "Microsoft Windows 11 Pro" 或 "Microsoft Windows 10 Pro"
		if strings.Contains(caption, "Windows 11") {
			return "Windows 11"
		}
		if strings.Contains(caption, "Windows 10") {
			return "Windows 10"
		}
		if strings.Contains(caption, "Windows 8") {
			return "Windows 8"
		}
		if strings.Contains(caption, "Windows 7") {
			return "Windows 7"
		}
	}

	return "Windows"
}

// GetArchInfo 获取架构信息（用于显示）
func GetArchInfo() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "ARM64"
	case "386":
		return "x86"
	default:
		return runtime.GOARCH
	}
}
