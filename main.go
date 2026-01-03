package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

// 全局变量，供 app.go 使用
var (
	mainApp    *application.App
	mainWindow *application.WebviewWindow
)

func main() {
	// 创建应用实例
	app := NewApp()

	// 创建 Wails v3 应用
	mainApp = application.New(application.Options{
		Name:        "awecloud-signaling",
		Description: "AWECloud Signaling Desktop Client",
		Services: []application.Service{
			application.NewService(app),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		Linux: application.LinuxOptions{
			DisableQuitOnLastWindowClosed: true,
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
		},
	})

	// 创建主窗口
	mainWindow = mainApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "awecloud-signaling",
		Width:     1024,
		Height:    768,
		MinWidth:  800,
		MinHeight: 600,
		BackgroundColour: application.RGBA{
			Red: 255, Green: 255, Blue: 255, Alpha: 255,
		},
		URL: "/",
	})

	// 设置窗口关闭事件 - 隐藏到托盘而不是关闭
	mainWindow.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		log.Printf("[Main] Window close event, hiding to tray")
		event.Cancel()
		mainWindow.Hide()
	})

	// 调用 startup
	app.startup()

	// 运行应用
	err := mainApp.Run()
	if err != nil {
		log.Fatal("Error:", err.Error())
	}
}
