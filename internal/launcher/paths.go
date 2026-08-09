package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type Paths struct {
	RootDir       string
	StateDir      string
	VersionsDir   string
	DownloadsDir  string
	CurrentFile   string
	PreviousFile  string
	UpdateTaskFile string
	HealthFile    string
}

func NewPaths(customRootDir string) (*Paths, error) {
	var root string
	if customRootDir != "" {
		root = customRootDir
	} else {
		userConfigDir, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("get user config dir failed: %w", err)
		}
		root = filepath.Join(userConfigDir, "beagle-signal")
	}

	stateDir := filepath.Join(root, "state")
	versionsDir := filepath.Join(root, "versions")
	downloadsDir := filepath.Join(root, "downloads")

	for _, dir := range []string{stateDir, versionsDir, downloadsDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("create dir %s failed: %w", dir, err)
		}
	}

	return &Paths{
		RootDir:        root,
		StateDir:       stateDir,
		VersionsDir:    versionsDir,
		DownloadsDir:   downloadsDir,
		CurrentFile:    filepath.Join(stateDir, "current.json"),
		PreviousFile:   filepath.Join(stateDir, "previous.json"),
		UpdateTaskFile: filepath.Join(stateDir, "update-task.json"),
		HealthFile:     filepath.Join(stateDir, "health.json"),
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
