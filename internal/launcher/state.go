package launcher

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/launcheripc"
)

func atomicWriteJSON(filePath string, v any) error {
	tmpPath := fmt.Sprintf("%s.tmp.%d", filePath, os.Getpid())
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state json failed: %w", err)
	}

	file, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create tmp state file failed: %w", err)
	}

	if _, err := file.Write(data); err != nil {
		file.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write tmp state file failed: %w", err)
	}

	if err := file.Sync(); err != nil {
		file.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("fsync tmp state file failed: %w", err)
	}
	file.Close()

	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename tmp state file failed: %w", err)
	}
	return nil
}

func readStateJSON[T any](filePath string, v *T) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	if err := launcheripc.DecodeStrictJSON(file, v); err != nil {
		// Corrupt state file recovery: move to .corrupt file
		corruptPath := fmt.Sprintf("%s.corrupt", filePath)
		_ = os.Rename(filePath, corruptPath)
		return fmt.Errorf("corrupt state file moved to %s: %w", corruptPath, err)
	}
	return nil
}

type CurrentInfo struct {
	SchemaVersion int                       `json:"schema_version"`
	Version       string                    `json:"version"`
	App           string                    `json:"app"`
	Artifact      launcheripc.ArtifactPayload `json:"artifact"`
	InstalledAt   string                    `json:"installed_at"`
}

type UpdateTaskState struct {
	SchemaVersion int                       `json:"schema_version"`
	OperationID   string                    `json:"operation_id"`
	RequestID     string                    `json:"request_id"`
	TaskID        *string                   `json:"task_id"`
	Source        string                    `json:"source"`
	TargetVersion string                    `json:"target_version"`
	TargetApp     string                    `json:"target_app"`
	Force         bool                      `json:"force"`
	Phase         string                    `json:"phase"`
	Progress      int                       `json:"progress"`
	Sequence      int64                     `json:"sequence"`
	Artifact      launcheripc.ArtifactPayload `json:"artifact"`
	CreatedAt     string                    `json:"created_at"`
	UpdatedAt     string                    `json:"updated_at"`
	Error         *launcheripc.ErrorDetail  `json:"error"`
}

type HealthInfo struct {
	SchemaVersion       int                      `json:"schema_version"`
	OperationID         string                   `json:"operation_id"`
	TargetVersion       string                   `json:"target_version"`
	TargetApp           string                   `json:"target_app"`
	PID                 int                      `json:"pid"`
	Status              string                   `json:"status"` // "observing", "healthy", "failed", "rolled_back"
	StartedAt           string                   `json:"started_at"`
	StartupDeadlineAt   string                   `json:"startup_deadline_at"`
	AppReadyAt          *string                  `json:"app_ready_at"`
	StabilityDeadlineAt *string                  `json:"stability_deadline_at"`
	LocallyHealthyAt    *string                  `json:"locally_healthy_at"`
	ServerHealthyAt     *string                  `json:"server_healthy_at"`
	Failure             *launcheripc.ErrorDetail `json:"failure"`
}

var _ = errors.New
var _ = io.EOF
