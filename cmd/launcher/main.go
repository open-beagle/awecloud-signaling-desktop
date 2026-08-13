package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/config"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/launcher"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/launcheripc"
)

func main() {
	if err := run(); err != nil {
		showStartupError(err.Error())
	}
}

func run() (runErr error) {
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
	defer func() {
		if runErr != nil {
			logger.Printf("Launcher failed: %v", runErr)
		}
	}()
	logger.Printf("Beagle Signal Launcher starting from %s", paths.RootDir)

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	updateServerAddress := launcher.ResolveUpdateServerAddress(cfg.ServerAddress)
	current, err := launcher.EnsureCurrentApp(context.Background(), paths, updateServerAddress, logger, showInitialInstallNotice)
	if err != nil {
		return err
	}

	coord, err := launcher.NewCoordinator(paths, updateServerAddress)
	if err != nil {
		return err
	}

	endpoint := os.Getenv(launcheripc.EnvLauncherEndpoint)
	if endpoint == "" {
		endpoint, err = launcheripc.NewEndpoint()
		if err != nil {
			return err
		}
	}
	token := os.Getenv(launcheripc.EnvLauncherToken)
	if token == "" {
		token, err = launcheripc.NewSessionToken()
		if err != nil {
			return err
		}
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
	processManager := launcher.NewProcessManager()
	for {
		appPath := filepath.Join(paths.VersionsDir, current.App)
		operationID := coord.OperationForApp(current)
		pid, err := processManager.StartApp(appPath, endpoint, token, current.Version, launcherPath, operationID)
		if err != nil {
			if coord.RollbackAfterFailure(&launcheripc.ErrorDetail{Code: launcheripc.ErrAppStartFailed, Message: err.Error()}) {
				current = coord.Current()
				continue
			}
			return err
		}
		coord.BeginAppRun(current, pid)
		logger.Printf("Desktop App version %s sha256 %s started with PID %d", current.Version, current.Artifact.SHA256, pid)
		waitCh := make(chan error, 1)
		go func() { waitCh <- processManager.Wait() }()
		select {
		case <-coord.UpdateAccepted():
			select {
			case exitErr := <-waitCh:
				if exitErr != nil {
					logger.Printf("Desktop App exited after update handoff: %v", exitErr)
				}
			case <-time.After(15 * time.Second):
				coord.CancelAcceptedUpdate(&launcheripc.ErrorDetail{Code: launcheripc.ErrAppUnstable, Message: "Desktop App did not exit within 15 seconds after update was accepted"})
				logger.Printf("Desktop App did not exit after update was accepted; update cancelled")
				if exitErr := <-waitCh; exitErr != nil {
					return fmt.Errorf("Desktop App exited unexpectedly after cancelled update: %w", exitErr)
				}
				return nil
			}
			if err := coord.ExecuteAcceptedUpdate(context.Background()); err != nil {
				logger.Printf("Desktop App update failed before switch: %v", err)
			}
			current = coord.Current()
			if current == nil {
				return fmt.Errorf("Desktop App state is unavailable after update execution")
			}
			continue
		case <-coord.RollbackRequested():
			processManager.StopApp()
			<-waitCh
			current = coord.Current()
			if current == nil {
				return fmt.Errorf("rolled back Desktop App state is unavailable")
			}
			continue
		case err := <-waitCh:
			if coord.HasAcceptedUpdate() {
				if executeErr := coord.ExecuteAcceptedUpdate(context.Background()); executeErr != nil {
					logger.Printf("Desktop App update failed before switch: %v", executeErr)
				}
				current = coord.Current()
				continue
			}
			if coord.RollbackAfterFailure(&launcheripc.ErrorDetail{Code: launcheripc.ErrAppUnstable, Message: "updated Desktop App exited before health confirmation"}) {
				current = coord.Current()
				continue
			}
			if err != nil {
				logger.Printf("Desktop App exited with error: %v", err)
				return fmt.Errorf("Desktop App exited unexpectedly: %w", err)
			}
			logger.Printf("Desktop App exited normally")
			return nil
		}
	}
}
