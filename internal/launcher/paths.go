package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type Paths struct {
	RootDir          string
	StateDir         string
	VersionsDir      string
	DownloadsDir     string
	LogsDir          string
	CurrentFile      string
	PreviousFile     string
	UpdateTaskFile   string
	HealthFile       string
	LauncherLogFile  string
	LauncherLockFile string
}

func NewPaths(customRootDir string) (*Paths, error) {
	var root string
	if customRootDir != "" {
		root = customRootDir
	} else {
		executablePath, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("get launcher executable path failed: %w", err)
		}
		if resolvedPath, resolveErr := filepath.EvalSymlinks(executablePath); resolveErr == nil {
			executablePath = resolvedPath
		}
		root = filepath.Dir(executablePath)
	}

	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve launcher root failed: %w", err)
	}
	root = filepath.Clean(absoluteRoot)

	stateDir := filepath.Join(root, "state")
	versionsDir := filepath.Join(root, "versions")
	downloadsDir := filepath.Join(root, "downloads")
	logsDir := filepath.Join(root, "logs")

	for _, dir := range []string{stateDir, versionsDir, downloadsDir, logsDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("create dir %s failed: %w", dir, err)
		}
	}

	return &Paths{
		RootDir:          root,
		StateDir:         stateDir,
		VersionsDir:      versionsDir,
		DownloadsDir:     downloadsDir,
		LogsDir:          logsDir,
		CurrentFile:      filepath.Join(stateDir, "current.json"),
		PreviousFile:     filepath.Join(stateDir, "previous.json"),
		UpdateTaskFile:   filepath.Join(stateDir, "update-task.json"),
		HealthFile:       filepath.Join(stateDir, "health.json"),
		LauncherLogFile:  filepath.Join(logsDir, "launcher.log"),
		LauncherLockFile: filepath.Join(stateDir, "launcher.lock"),
	}, nil
}

func (p *Paths) LogicalAppName(version string) string {
	ext := ".app"
	if runtime.GOOS == "windows" {
		ext = ".app.exe"
	}
	return fmt.Sprintf("beagle-signal-%s%s", version, ext)
}

func (p *Paths) AppPath(version string) string {
	return filepath.Join(p.VersionsDir, p.LogicalAppName(version))
}
