package device

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"runtime"
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
	// 使用环境变量或简单的方法检测
	// 在Windows上，可以通过多种方式检测版本
	// 这里使用一个简化的实现

	// 尝试读取环境变量
	if osVer := os.Getenv("OS"); osVer != "" {
		// 默认返回Windows 10（大多数现代系统）
		return "Windows 10"
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
