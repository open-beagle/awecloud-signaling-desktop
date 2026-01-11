package version

import (
	"fmt"
	"os"
)

var (
	// 这些变量在编译时通过 -ldflags 注入
	Version     = "dev"
	GitCommit   = "unknown"
	BuildTime   = "unknown"
	BuildNumber = "0"
)

func init() {
	// 如果是 dev 版本，尝试从环境变量读取版本号
	if Version == "dev" {
		if envVersion := os.Getenv("APP_VERSION"); envVersion != "" {
			Version = envVersion
		}
	}
}

// GetVersion 获取版本号
func GetVersion() string {
	return Version
}

// GetFullVersion 获取完整版本信息
func GetFullVersion() string {
	return fmt.Sprintf("%s (commit: %s, built: %s, build: %s)",
		Version, GitCommit, BuildTime, BuildNumber)
}

// GetBuildNumber 获取构建号
func GetBuildNumber() string {
	return BuildNumber
}
