package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/launcheripc"
)

type ProcessManager struct {
	mu  sync.Mutex
	cmd *exec.Cmd
}

func NewProcessManager() *ProcessManager {
	return &ProcessManager{}
}

func (pm *ProcessManager) StartApp(appPath, endpoint, token, expectedVersion, launcherPath, operationID string) (int, error) {
	if _, err := os.Stat(appPath); err != nil {
		return 0, fmt.Errorf("app executable not found: %w", err)
	}

	cmd := exec.Command(appPath)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("%s=%s", launcheripc.EnvLauncherEndpoint, endpoint),
		fmt.Sprintf("%s=%s", launcheripc.EnvLauncherToken, token),
		fmt.Sprintf("%s=%s", launcheripc.EnvAppVersion, expectedVersion),
		fmt.Sprintf("%s=%s", launcheripc.EnvLauncherPath, launcherPath),
		fmt.Sprintf("%s=%d", launcheripc.EnvIPCVersion, launcheripc.IPCVersion),
		fmt.Sprintf("%s=%s", launcheripc.EnvUpdateOperationID, operationID),
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("cmd.Start failed: %w", err)
	}

	pm.mu.Lock()
	pm.cmd = cmd
	pm.mu.Unlock()
	return cmd.Process.Pid, nil
}

func (pm *ProcessManager) IsRunning() bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if pm.cmd == nil || pm.cmd.Process == nil {
		return false
	}
	// Check if process has exited
	if pm.cmd.ProcessState != nil && pm.cmd.ProcessState.Exited() {
		return false
	}
	return true
}

func (pm *ProcessManager) Wait() error {
	pm.mu.Lock()
	if pm.cmd == nil {
		pm.mu.Unlock()
		return fmt.Errorf("Desktop App has not been started")
	}
	cmd := pm.cmd
	pm.mu.Unlock()
	err := cmd.Wait()
	pm.mu.Lock()
	if pm.cmd == cmd {
		pm.cmd = nil
	}
	pm.mu.Unlock()
	return err
}

func (pm *ProcessManager) StopApp() {
	pm.mu.Lock()
	cmd := pm.cmd
	pm.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
