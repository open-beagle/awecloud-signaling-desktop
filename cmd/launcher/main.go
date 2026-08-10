package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/config"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/launcher"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/launcheripc"
)

func main() {
	if err := run(); err != nil {
		log.Printf("Launcher failed: %v", err)
		showStartupError(err.Error())
	}
}

func run() error {
	paths, err := launcher.NewPaths("")
	if err != nil {
		return err
	}
	instanceLock, err := launcher.AcquireInstanceLock(paths.LauncherLockFile)
	if err != nil {
		return err
	}
	defer instanceLock.Release()

	logFile, err := os.OpenFile(paths.LauncherLogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	logger := log.New(io.MultiWriter(logFile, os.Stderr), "", log.Ldate|log.Ltime|log.Lmicroseconds)
	log.SetOutput(io.MultiWriter(logFile, os.Stderr))
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	logger.Printf("Beagle Signal Launcher starting from %s", paths.RootDir)

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	current, err := launcher.EnsureCurrentApp(context.Background(), paths, cfg.ServerAddress, logger, showInitialInstallNotice)
	if err != nil {
		return err
	}

	coord, err := launcher.NewCoordinator(paths)
	if err != nil {
		return err
	}

	endpoint, err := launcheripc.NewEndpoint()
	if err != nil {
		return err
	}
	token, err := launcheripc.NewSessionToken()
	if err != nil {
		return err
	}

	ipcServer := launcheripc.NewServer(endpoint, token, launcheripc.ExpectedPeerUID(), coord)
	if err := ipcServer.Start(); err != nil {
		return err
	}
	defer ipcServer.Stop()
	logger.Printf("Launcher IPC server started")

	launcherPath, err := os.Executable()
	if err != nil {
		return err
	}
	if resolvedPath, resolveErr := filepath.EvalSymlinks(launcherPath); resolveErr == nil {
		launcherPath = resolvedPath
	}
	appPath := paths.AppPath(current.Version)
	processManager := launcher.NewProcessManager()
	pid, err := processManager.StartApp(appPath, endpoint, token, current.Version, launcherPath)
	if err != nil {
		return err
	}
	logger.Printf("Desktop App version %s started with PID %d", current.Version, pid)

	if err := processManager.Wait(); err != nil {
		logger.Printf("Desktop App exited with error: %v", err)
		return fmt.Errorf("Desktop App exited unexpectedly: %w", err)
	}
	logger.Printf("Desktop App exited normally")
	return nil
}
