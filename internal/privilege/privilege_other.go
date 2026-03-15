//go:build !darwin

// Package privilege 提供平台提权工具
// 非 macOS 平台不需要 osascript 提权
package privilege

// RunWithPrivilege 非 macOS 平台不需要提权
func RunWithPrivilege(command string) (string, error) {
	return "", nil
}

// RunBatch 非 macOS 平台不需要提权
func RunBatch(commands []string) error {
	return nil
}
