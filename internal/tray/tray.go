package tray

import (
	"context"
	_ "embed"
	goruntime "runtime"

	"github.com/energye/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed icon.ico
var iconIco []byte

//go:embed icon.png
var iconPng []byte

// Manager 系统托盘管理器
type Manager struct {
	ctx      context.Context
	quitFunc func()
}

// NewManager 创建托盘管理器
func NewManager(ctx context.Context, quitFunc func()) *Manager {
	return &Manager{
		ctx:      ctx,
		quitFunc: quitFunc,
	}
}

// Start 启动系统托盘
func (m *Manager) Start() {
	go systray.Run(m.onReady, m.onExit)
}

// onReady 托盘就绪回调
func (m *Manager) onReady() {
	// Windows 使用 ico 格式，其他平台使用 png
	if goruntime.GOOS == "windows" {
		systray.SetIcon(iconIco)
	} else {
		systray.SetIcon(iconPng)
	}
	systray.SetTitle("AWECloud Signaling")
	systray.SetTooltip("AWECloud Signaling Desktop")

	// 添加菜单项
	showItem := systray.AddMenuItem("显示窗口", "显示主窗口")
	systray.AddSeparator()
	exitItem := systray.AddMenuItem("退出", "退出应用")

	// 单击托盘图标显示窗口
	systray.SetOnClick(func(menu systray.IMenu) {
		m.ShowWindow()
	})

	// 双击托盘图标显示窗口
	systray.SetOnDClick(func(menu systray.IMenu) {
		m.ShowWindow()
	})

	// 右键显示菜单
	systray.SetOnRClick(func(menu systray.IMenu) {
		menu.ShowMenu()
	})

	// 菜单项点击事件
	showItem.Click(func() {
		m.ShowWindow()
	})

	exitItem.Click(func() {
		m.Quit()
	})
}

// onExit 托盘退出回调
func (m *Manager) onExit() {
	// 清理资源
}

// ShowWindow 显示窗口
func (m *Manager) ShowWindow() {
	runtime.WindowShow(m.ctx)
}

// HideWindow 隐藏窗口
func (m *Manager) HideWindow() {
	runtime.WindowHide(m.ctx)
}

// Quit 退出应用
func (m *Manager) Quit() {
	systray.Quit()
	if m.quitFunc != nil {
		m.quitFunc()
	}
}
