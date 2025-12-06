package version

import (
	"fmt"
)

var (
	// 这些变量在编译时通过 -ldflags 注入
	Version     = "dev"
	GitCommit   = "unknown"
	BuildTime   = "unknown"
	BuildNumber = "0"
)

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
