package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/launcheripc"
)

type ProcessManager struct {
	cmd *exec.Cmd
}

func NewProcessManager() *ProcessManager {
	return &ProcessManager{}
}

func (pm *ProcessManager) StartApp(appPath, endpoint, token, expectedVersion, launcherPath string) (int, error) {
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
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("cmd.Start failed: %w", err)
	}

	pm.cmd = cmd
	return cmd.Process.Pid, nil
}

func (pm *ProcessManager) IsRunning() bool {
	if pm.cmd == nil || pm.cmd.Process == nil {
		return false
	}
	// Check if process has exited
	if pm.cmd.ProcessState != nil && pm.cmd.ProcessState.Exited() {
		return false
	}
	return true
}

func (pm *ProcessManager) StopApp() {
	if pm.cmd != nil && pm.cmd.Process != nil {
		_ = pm.cmd.Process.Kill()
		_ = pm.cmd.Wait()
	}
}

var _ = time.Second
