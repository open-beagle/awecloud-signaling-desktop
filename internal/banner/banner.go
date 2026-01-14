package banner

import (
	"log"
	"runtime"
	"strings"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/version"
)

// Print 打印启动横幅
func Print() {
	width := 60

	// 顶部边框
	log.Println(strings.Repeat("=", width))

	// 应用名称（居中）
	appName := "AWECloud Signaling Desktop"
	printCentered(appName, width)

	// 分隔线
	log.Println(strings.Repeat("-", width))

	// 版本信息（左对齐）
	printKeyValue("Version", version.Version)
	printKeyValue("Git Commit", version.GitCommit)
	printKeyValue("Build Date", version.BuildTime)
	printKeyValue("Go Version", runtime.Version())

	// 底部边框
	log.Println(strings.Repeat("=", width))
}

// printCentered 打印居中文本
func printCentered(text string, width int) {
	padding := (width - len(text)) / 2
	if padding < 0 {
		padding = 0
	}
	log.Printf("%s%s", strings.Repeat(" ", padding), text)
}

// printKeyValue 打印键值对
func printKeyValue(key, value string) {
	log.Printf("  %s: %s", key, value)
}
