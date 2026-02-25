package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/config"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/singleton"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/telemetry"
	appVersion "github.com/open-beagle/awecloud-signaling-desktop/internal/version"
)

//go:embed frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

// 全局变量，供 app.go 使用
var (
	mainApp    *application.App
	mainWindow *application.WebviewWindow
)

func main() {
	// Linux 环境下设置环境变量，跳过 WebView 的 TLS 证书验证
	if runtime.GOOS == "linux" {
		os.Setenv("WEBKIT_IGNORE_TLS_ERRORS", "1")
		log.Printf("[Main] Set WEBKIT_IGNORE_TLS_ERRORS=1 for Linux WebView")
	}

	// 单实例检查
	if !singleton.CheckSingleInstance() {
		log.Println("应用已在运行中，退出当前实例")
		os.Exit(0)
	}
	defer singleton.ReleaseSingleInstance()

	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Failed to load config: %v", err)
		cfg = &config.Config{
			Telemetry: config.TelemetryConfig{
				Name:      "signaling-desktop",
				Namespace: "default",
				Cluster:   "default",
			},
		}
	}

	// 设置 telemetry 日志记录器
	telemetry.SetLogger(&telemetryLogger{})

	// 初始化 OpenTelemetry
	if err := telemetry.Init(telemetry.Config{
		Endpoint:    cfg.Telemetry.Endpoint,
		ServiceName: cfg.Telemetry.Name,
		Namespace:   cfg.Telemetry.Namespace,
		Cluster:     cfg.Telemetry.Cluster,
	}, &telemetry.BuildInfo{
		Version:   appVersion.Version,
		GitCommit: appVersion.GitCommit,
		BuildDate: appVersion.BuildTime,
		GoVersion: "go1.25+",
	}, &telemetry.ProcessAttributes{
		User: cfg.ClientID, // 使用 ClientID 作为用户标识
	}); err != nil {
		log.Printf("Warning: Failed to initialize OpenTelemetry: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := telemetry.Shutdown(ctx); err != nil {
			log.Printf("Warning: Failed to shutdown OpenTelemetry: %v", err)
		}
	}()

	// 创建应用实例
	app := NewApp()

	// 从 embed.FS 中提取 frontend/dist 子目录
	frontendFS, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		log.Fatalf("Failed to get frontend/dist from embedded assets: %v", err)
	}

	// 创建 Wails v3 应用
	mainApp = application.New(application.Options{
		Name:        "awecloud-signaling",
		Description: "AWECloud Signaling Desktop Client",
		Icon:        appIcon,
		Services: []application.Service{
			application.NewService(app),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(frontendFS),
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
	err = mainApp.Run()
	if err != nil {
		log.Fatal("Error:", err.Error())
	}
}

// telemetryLogger 实现 telemetry.Logger 接口
type telemetryLogger struct{}

func (l *telemetryLogger) Info(args ...interface{}) {
	log.Println(args...)
}

func (l *telemetryLogger) Infof(format string, args ...interface{}) {
	log.Printf(format, args...)
}

func (l *telemetryLogger) Warn(args ...interface{}) {
	log.Println(args...)
}

func (l *telemetryLogger) Warnf(format string, args ...interface{}) {
	log.Printf(format, args...)
}
