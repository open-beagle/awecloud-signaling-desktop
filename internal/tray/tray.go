// Package tray 提供系统托盘相关的资源
// 注意：Wails v3 使用原生系统托盘支持，托盘逻辑已移至 app.go
package tray

import (
	_ "embed"
)

// 保留图标资源供其他地方使用（如果需要）
//
//go:embed icon.ico
var IconIco []byte

//go:embed icon.png
var IconPng []byte
