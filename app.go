package main

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/client"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/config"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/models"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/tailscale"
	appVersion "github.com/open-beagle/awecloud-signaling-desktop/internal/version"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// App struct
type App struct {
	desktopClient *client.DesktopClient
	tsManager     *tailscale.Manager
	commandChan   chan *models.VisitorCommand
	statusChan    chan *models.VisitorStatus
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		commandChan: make(chan *models.VisitorCommand, 10),
		statusChan:  make(chan *models.VisitorStatus, 10),
	}
}

func (a *App) startup() {
	log.SetOutput(&logWriter{})
	log.SetFlags(log.Ltime)

	cfg, err := config.Load()
	if err != nil {
		log.Printf("Failed to load config: %v", err)
		return
	}

	config.GlobalConfig = cfg
	log.Printf("Using server address: %s", config.GlobalConfig.ServerAddress)
	log.Printf("Desktop app started")
	log.Printf("Version: %s, Build: %s, Commit: %s", appVersion.Version, appVersion.BuildNumber, appVersion.GitCommit)

	a.setupSystemTray()
	log.Printf("System tray started")
}

func (a *App) setupSystemTray() {
	if mainApp == nil {
		log.Printf("[App] mainApp is nil, cannot setup system tray")
		return
	}

	systray := mainApp.SystemTray.New()
	systray.SetIcon(appIcon)

	menu := mainApp.NewMenu()
	menu.Add("显示窗口").OnClick(func(ctx *application.Context) {
		if mainWindow != nil {
			mainWindow.Show()
			mainWindow.Focus()
		}
	})
	menu.AddSeparator()
	menu.Add("退出").OnClick(func(ctx *application.Context) {
		a.shutdown()
		mainApp.Quit()
	})
	systray.SetMenu(menu)

	systray.OnClick(func() {
		if mainWindow != nil {
			mainWindow.Show()
			mainWindow.Focus()
		}
	})

	systray.OnRightClick(func() {
		systray.OpenMenu()
	})
}

func (a *App) shutdown() {
	if a.desktopClient != nil {
		a.desktopClient.Stop()
	}
	if a.tsManager != nil {
		a.tsManager.Disconnect()
	}
	log.Printf("Desktop app shutdown")
}

func (a *App) Login(serverAddr, clientID, clientSecret string, rememberMe bool) error {
	log.Printf("[App] Login: serverAddr=%s, clientID=%s, rememberMe=%v", serverAddr, clientID, rememberMe)

	config.GlobalConfig.ServerAddress = serverAddr
	config.GlobalConfig.ClientID = clientID
	config.GlobalConfig.RememberMe = rememberMe

	log.Printf("[App] Creating Desktop-Web client for: %s", serverAddr)
	a.desktopClient = client.NewDesktopClient(serverAddr, a.commandChan, a.statusChan)
	if err := a.desktopClient.Start(); err != nil {
		return fmt.Errorf("failed to start desktop client: %w", err)
	}
	log.Printf("[App] Desktop-Web client started successfully")

	var authResult *client.AuthResult
	var err error

	if clientSecret == "" && config.GlobalConfig.HasValidToken() {
		log.Printf("Attempting token authentication...")
		authResult, err = a.desktopClient.AuthWithToken(config.GlobalConfig.ClientID, config.GlobalConfig.DeviceToken)
	} else {
		log.Printf("Attempting secret authentication...")
		authResult, err = a.desktopClient.AuthWithSecret(clientID, clientSecret, rememberMe)
	}

	if err != nil {
		a.desktopClient.Stop()
		a.desktopClient = nil
		return fmt.Errorf("authentication failed: %w", err)
	}

	log.Printf("Config: Server=%s, Token length=%d",
		config.GlobalConfig.ServerAddress, len(config.GlobalConfig.DeviceToken))
	log.Printf("Authentication successful: %s", authResult.Message)
	log.Printf("Login successful, Tailscale will be initialized when connecting to services")

	return nil
}

func (a *App) initializeTailscale() error {
	if a.tsManager != nil && a.tsManager.IsConnected() {
		log.Printf("[App] Tailscale already connected")
		return nil
	}

	log.Printf("[App] Initializing Tailscale client...")

	tsAuth, err := a.desktopClient.GetTailscaleAuth()
	if err != nil {
		log.Printf("[App] Failed to get Tailscale auth: %v", err)
		return fmt.Errorf("获取 Tailscale 认证失败: %w", err)
	}

	log.Printf("[App] Tailscale auth received: control_url=%s", tsAuth.ControlURL)

	a.tsManager = tailscale.NewManager()

	hostname := fmt.Sprintf("desktop-%s", config.GlobalConfig.ClientID)
	if err := a.tsManager.Connect(tsAuth.ControlURL, tsAuth.AuthKey, hostname); err != nil {
		a.tsManager = nil
		return fmt.Errorf("连接 Tailscale 失败: %w", err)
	}

	log.Printf("[App] Tailscale connected, IP: %s", a.tsManager.GetIP())
	return nil
}

func (a *App) Logout() {
	log.Printf("[App] Logout called")

	if a.desktopClient != nil {
		a.desktopClient.Stop()
		a.desktopClient = nil
	}
	if a.tsManager != nil {
		a.tsManager.Disconnect()
		a.tsManager = nil
	}

	if config.GlobalConfig.RememberMe {
		config.GlobalConfig.ClearToken()
		if err := config.GlobalConfig.Save(); err != nil {
			log.Printf("[App] Failed to save config after logout: %v", err)
		} else {
			log.Printf("[App] Token cleared, config saved")
		}
	} else {
		if err := config.Delete(); err != nil {
			log.Printf("[App] Failed to delete config after logout: %v", err)
		} else {
			log.Printf("[App] Config deleted")
		}
	}
}

func (a *App) GetServices() ([]*models.ServiceInfo, error) {
	log.Printf("[App] GetServices called")

	if a.desktopClient == nil {
		log.Printf("[App] GetServices error: not logged in")
		return nil, fmt.Errorf("not logged in")
	}

	log.Printf("[App] Calling desktopClient.GetServices()")
	services, err := a.desktopClient.GetServices()
	if err != nil {
		log.Printf("[App] GetServices error: %v", err)
		return nil, err
	}

	log.Printf("[App] Got %d services from desktopClient", len(services))

	favorites, err := a.desktopClient.GetFavorites()
	if err != nil {
		log.Printf("[App] Failed to get favorites: %v", err)
	} else {
		log.Printf("[App] Got %d favorites from server", len(favorites))
		favoriteMap := make(map[int64]bool)
		favoritePortMap := make(map[int64]int)
		for _, fav := range favorites {
			log.Printf("[App] Favorite: instance_id=%d, local_port=%d", fav.STCPInstanceID, fav.LocalPort)
			favoriteMap[fav.STCPInstanceID] = true
			if fav.LocalPort > 0 {
				favoritePortMap[fav.STCPInstanceID] = fav.LocalPort
			}
		}
		for _, service := range services {
			service.IsFavorite = favoriteMap[service.InstanceID]
			if port, ok := favoritePortMap[service.InstanceID]; ok {
				service.PreferredPort = port
			}
		}
		log.Printf("[App] Marked %d services as favorites with port preferences", len(favorites))
	}

	log.Printf("[App] Returning %d services", len(services))
	return services, nil
}

func (a *App) ToggleFavorite(instanceID int64, localPort int) (bool, error) {
	if a.desktopClient == nil {
		return false, fmt.Errorf("not logged in")
	}

	isFavorite, err := a.desktopClient.ToggleFavorite(instanceID, localPort)
	if err != nil {
		log.Printf("[App] Failed to toggle favorite for instance %d: %v", instanceID, err)
		return false, err
	}

	log.Printf("[App] Toggled favorite for instance %d: is_favorite=%v, port=%d", instanceID, isFavorite, localPort)
	return isFavorite, nil
}

func (a *App) UpdateFavoritePort(instanceID int64, localPort int) error {
	if a.desktopClient == nil {
		return fmt.Errorf("not logged in")
	}

	err := a.desktopClient.UpdateFavoritePort(instanceID, localPort)
	if err != nil {
		log.Printf("[App] Failed to update favorite port for instance %d: %v", instanceID, err)
		return err
	}

	log.Printf("[App] Updated favorite port for instance %d: port=%d", instanceID, localPort)
	return nil
}

func (a *App) ConnectService(instanceID int64, localPort int) error {
	if a.desktopClient == nil {
		return fmt.Errorf("not logged in")
	}

	if a.tsManager == nil || !a.tsManager.IsConnected() {
		log.Printf("[App] Initializing Tailscale for first connection")
		if err := a.initializeTailscale(); err != nil {
			return fmt.Errorf("failed to initialize Tailscale: %w", err)
		}
	}

	if err := a.desktopClient.ConnectService(instanceID, localPort); err != nil {
		return err
	}

	log.Printf("[App] Connected to service %d on port %d", instanceID, localPort)

	services, err := a.GetServices()
	if err != nil {
		log.Printf("[App] Failed to get services for port update check: %v", err)
		return nil
	}

	for _, service := range services {
		if service.InstanceID == instanceID && service.IsFavorite && service.PreferredPort != localPort {
			go func() {
				if err := a.UpdateFavoritePort(instanceID, localPort); err != nil {
					log.Printf("[App] Failed to update favorite port: %v", err)
				}
			}()
			break
		}
	}

	return nil
}

func (a *App) DisconnectService(instanceID int64) error {
	if a.desktopClient == nil {
		return fmt.Errorf("not logged in")
	}
	return a.desktopClient.DisconnectService(instanceID)
}

func (a *App) IsAuthenticated() bool {
	return a.desktopClient != nil && a.desktopClient.IsAuthenticated()
}

func (a *App) GetConfig() *config.Config {
	return config.GlobalConfig
}

type VersionInfo struct {
	Version     string `json:"version"`
	GitCommit   string `json:"gitCommit"`
	BuildDate   string `json:"buildDate"`
	BuildNumber string `json:"buildNumber"`
}

func (a *App) GetVersion() *VersionInfo {
	return &VersionInfo{
		Version:     appVersion.Version,
		GitCommit:   appVersion.GitCommit,
		BuildDate:   appVersion.BuildTime,
		BuildNumber: appVersion.BuildNumber,
	}
}

func (a *App) CheckVersion(serverAddr string) (*client.VersionCheckResponse, error) {
	log.Printf("[App] CheckVersion called for server: %s", serverAddr)
	return client.CheckVersion(serverAddr)
}

func (a *App) GetWindowTitle() string {
	if appVersion.BuildNumber != "0" && appVersion.BuildNumber != "" {
		return fmt.Sprintf("awecloud-signaling  %s (Build %s)", appVersion.Version, appVersion.BuildNumber)
	}
	return fmt.Sprintf("awecloud-signaling  %s", appVersion.Version)
}

type SavedCredentials struct {
	ServerAddress string `json:"server_address"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
	RememberMe    bool   `json:"remember_me"`
	HasToken      bool   `json:"has_token"`
	IsOnline      bool   `json:"is_online"`
}

func (a *App) CheckSavedCredentials() *SavedCredentials {
	log.Printf("[App] CheckSavedCredentials: HasToken=%v", config.GlobalConfig.HasValidToken())

	serverAddr := config.GlobalConfig.ServerAddress

	if config.GlobalConfig.ClientID == "" {
		return &SavedCredentials{
			ServerAddress: serverAddr,
			ClientID:      "",
			ClientSecret:  "",
			RememberMe:    true,
			HasToken:      false,
			IsOnline:      false,
		}
	}

	hasToken := config.GlobalConfig.HasValidToken()
	isOnline := client.CanConnectToServer(config.GlobalConfig.ServerAddress)
	log.Printf("[App] Token status: HasToken=%v, Server online: %v", hasToken, isOnline)

	return &SavedCredentials{
		ServerAddress: serverAddr,
		ClientID:      config.GlobalConfig.ClientID,
		ClientSecret:  "",
		RememberMe:    true,
		HasToken:      hasToken,
		IsOnline:      isOnline,
	}
}

func (a *App) ClearCredentials() error {
	config.GlobalConfig.ClearToken()
	config.GlobalConfig.ClientID = ""
	config.GlobalConfig.ServerAddress = ""
	return config.GlobalConfig.Save()
}

func (a *App) GetLogs() []string {
	return GetRecentLogs(100)
}

type DeviceInfo struct {
	DeviceToken string `json:"device_token"`
	DeviceName  string `json:"device_name"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	Hostname    string `json:"hostname"`
	Status      string `json:"status"`
	LastUsedAt  string `json:"last_used_at"`
	CreatedAt   string `json:"created_at"`
	IsCurrent   bool   `json:"is_current"`
}

func (a *App) GetDevices() ([]*DeviceInfo, error) {
	if a.desktopClient == nil {
		return nil, fmt.Errorf("not logged in")
	}

	clientDevices, err := a.desktopClient.GetDevices()
	if err != nil {
		return nil, err
	}

	devices := make([]*DeviceInfo, 0, len(clientDevices))
	for _, d := range clientDevices {
		devices = append(devices, &DeviceInfo{
			DeviceToken: d.DeviceToken,
			DeviceName:  d.DeviceName,
			OS:          d.OS,
			Arch:        d.Arch,
			Hostname:    d.Hostname,
			Status:      d.Status,
			LastUsedAt:  d.LastUsedAt,
			CreatedAt:   d.CreatedAt,
			IsCurrent:   d.IsCurrent,
		})
	}

	return devices, nil
}

func (a *App) OfflineDevice(deviceToken string) error {
	if a.desktopClient == nil {
		return fmt.Errorf("not logged in")
	}
	log.Printf("Offline device: %s", deviceToken)
	return a.desktopClient.OfflineDevice(deviceToken)
}

func (a *App) DeleteDevice(deviceToken string) error {
	if a.desktopClient == nil {
		return fmt.Errorf("not logged in")
	}
	log.Printf("Delete device: %s", deviceToken)
	return a.desktopClient.DeleteDevice(deviceToken)
}

var (
	logBuffer   []string
	logMutex    sync.Mutex
	maxLogLines = 5000
)

func LogToBuffer(message string) {
	logMutex.Lock()
	defer logMutex.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logLine := fmt.Sprintf("[%s] %s", timestamp, message)

	logBuffer = append(logBuffer, logLine)
	if len(logBuffer) > maxLogLines {
		logBuffer = logBuffer[len(logBuffer)-maxLogLines:]
	}

	log.Println(message)
}

func GetRecentLogs(n int) []string {
	logMutex.Lock()
	defer logMutex.Unlock()

	if n > len(logBuffer) {
		n = len(logBuffer)
	}

	if n == 0 {
		return []string{}
	}

	return logBuffer[len(logBuffer)-n:]
}

type logWriter struct{}

func (w *logWriter) Write(p []byte) (n int, err error) {
	logMutex.Lock()
	defer logMutex.Unlock()

	message := strings.TrimSuffix(string(p), "\n")
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logLine := fmt.Sprintf("[%s] %s", timestamp, message)

	logBuffer = append(logBuffer, logLine)
	if len(logBuffer) > maxLogLines {
		logBuffer = logBuffer[len(logBuffer)-maxLogLines:]
	}

	fmt.Println(logLine)
	return len(p), nil
}

func (a *App) HideToTray() {
	log.Printf("[App] HideToTray called")
	if mainWindow != nil {
		mainWindow.Hide()
	}
}

func (a *App) ShowFromTray() {
	log.Printf("[App] ShowFromTray called")
	if mainWindow != nil {
		mainWindow.Show()
		mainWindow.Focus()
	}
}

func (a *App) QuitApp() {
	log.Printf("[App] QuitApp called")
	a.shutdown()
	if mainApp != nil {
		mainApp.Quit()
	}
}
