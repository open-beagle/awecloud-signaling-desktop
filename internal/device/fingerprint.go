package device

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/denisbrodbeck/machineid"
	"github.com/shirou/gopsutil/host"
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
	// 使用 gopsutil 获取系统信息（跨平台）
	info, err := host.Info()
	if err != nil {
		// 如果获取失败，使用 runtime 的基本信息
		return getFallbackOSInfo()
	}

	// 根据平台返回友好的名称
	switch runtime.GOOS {
	case "windows":
		return getWindowsVersionFromInfo(info)
	case "darwin":
		return getMacOSVersionFromInfo(info)
	case "linux":
		return getLinuxVersionFromInfo(info)
	default:
		return info.Platform
	}
}

// getWindowsVersionFromInfo 从 host.Info 获取 Windows 版本
func getWindowsVersionFromInfo(info *host.InfoStat) string {
	// Windows 11 的判断：Build 号 >= 22000
	// info.PlatformVersion 格式如 "10.0.22621"
	parts := strings.Split(info.PlatformVersion, ".")
	if len(parts) >= 3 {
		// 第三部分是 Build 号
		if buildStr := parts[2]; buildStr != "" {
			// 简单判断：如果 Build 号以 "22" 或更大开头，就是 Windows 11
			if len(buildStr) >= 2 && buildStr[:2] >= "22" {
				return "Windows 11"
			}
		}
	}

	// 检查 PlatformFamily 和 Platform
	if strings.Contains(info.Platform, "11") {
		return "Windows 11"
	}
	if strings.Contains(info.Platform, "10") {
		return "Windows 10"
	}

	// 默认返回 Windows + 版本号
	if info.PlatformVersion != "" {
		return fmt.Sprintf("Windows %s", info.PlatformVersion)
	}

	return "Windows"
}

// getMacOSVersionFromInfo 从 host.Info 获取 macOS 版本
func getMacOSVersionFromInfo(info *host.InfoStat) string {
	// macOS 版本号如 "14.1.1" (Sonoma)
	if info.PlatformVersion != "" {
		// 提取主版本号
		parts := strings.Split(info.PlatformVersion, ".")
		if len(parts) > 0 {
			majorVersion := parts[0]
			// macOS 版本名称映射
			versionName := getMacOSVersionName(majorVersion)
			if versionName != "" {
				return fmt.Sprintf("macOS %s (%s)", info.PlatformVersion, versionName)
			}
			return fmt.Sprintf("macOS %s", info.PlatformVersion)
		}
	}
	return "macOS"
}

// getMacOSVersionName 获取 macOS 版本代号
func getMacOSVersionName(majorVersion string) string {
	versionMap := map[string]string{
		"15": "Sequoia",
		"14": "Sonoma",
		"13": "Ventura",
		"12": "Monterey",
		"11": "Big Sur",
		"10": "Catalina",
	}
	return versionMap[majorVersion]
}

// getLinuxVersionFromInfo 从 host.Info 获取 Linux 版本
func getLinuxVersionFromInfo(info *host.InfoStat) string {
	// Linux 返回发行版信息
	if info.Platform != "" && info.PlatformVersion != "" {
		return fmt.Sprintf("%s %s", info.Platform, info.PlatformVersion)
	}
	if info.Platform != "" {
		return info.Platform
	}
	return "Linux"
}

// getFallbackOSInfo 获取备用的操作系统信息
func getFallbackOSInfo() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows"
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	default:
		return runtime.GOOS
	}
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
