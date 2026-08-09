package launcheripc

import "time"

const (
	EnvLauncherEndpoint = "BEAGLE_SIGNAL_LAUNCHER_ENDPOINT"
	EnvLauncherToken    = "BEAGLE_SIGNAL_LAUNCHER_TOKEN"
	EnvAppVersion       = "BEAGLE_SIGNAL_APP_VERSION"
	EnvLauncherPath     = "BEAGLE_SIGNAL_LAUNCHER_PATH"
	EnvIPCVersion       = "BEAGLE_SIGNAL_IPC_VERSION"
)

const (
	SchemaVersion = 1
	IPCVersion    = 1
	HostHeader    = "launcher.local"
)

const (
	ErrInvalidRequest           = "invalid_request"
	ErrUnsupportedSchema        = "unsupported_schema"
	ErrUnsupportedIPCVersion   = "unsupported_ipc_version"
	ErrIPCConnectTimeout        = "ipc_connect_timeout"
	ErrSessionMismatch          = "session_mismatch"
	ErrConcurrentPoll           = "concurrent_poll"
	ErrIPCRecoveryFailed        = "ipc_recovery_failed"
	ErrLauncherUpdateRequired   = "launcher_update_required"
	ErrUpdateInProgress         = "update_in_progress"
	ErrDownloadFailed           = "download_failed"
	ErrArtifactSizeMismatch     = "artifact_size_mismatch"
	ErrChecksumMismatch         = "checksum_mismatch"
	ErrSignatureInvalid         = "signature_invalid"
	ErrPlatformSignatureInvalid = "platform_signature_invalid"
	ErrArchiveInvalid           = "archive_invalid"
	ErrVersionConflict          = "version_conflict"
	ErrInstallFailed            = "install_failed"
	ErrAppStartFailed           = "app_start_failed"
	ErrAppReadyTimeout          = "app_ready_timeout"
	ErrAppVersionMismatch       = "app_version_mismatch"
	ErrAppUnstable              = "app_unstable"
	ErrRollbackFailed           = "rollback_failed"
)

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type CommonResponse struct {
	SchemaVersion int          `json:"schema_version"`
	OK            bool         `json:"ok"`
	Data          any          `json:"data,omitempty"`
	Error         *ErrorDetail `json:"error,omitempty"`
}

type ConnectRequest struct {
	SchemaVersion    int    `json:"schema_version"`
	IPCVersion       int    `json:"ipc_version"`
	PID              int    `json:"pid"`
	Version          string `json:"version"`
	ProcessStartedAt string `json:"process_started_at"`
}

type UpdateSnapshot struct {
	OperationID string       `json:"operation_id"`
	Phase       string       `json:"phase"`
	Progress    int          `json:"progress"`
	Error       *ErrorDetail `json:"error"`
}

type ConnectResponseData struct {
	SessionID       string          `json:"session_id"`
	LauncherPID     int             `json:"launcher_pid"`
	ExpectedVersion string          `json:"expected_version"`
	Recovered       bool            `json:"recovered"`
	NextEventSeq    int64           `json:"next_event_sequence"`
	Update          *UpdateSnapshot `json:"update"`
}

type ArtifactPayload struct {
	ID          string `json:"id"`
	Role        string `json:"role"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	PackageType string `json:"package_type"`
	Filename    string `json:"filename"`
	DownloadURL string `json:"download_url"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	Signature   string `json:"signature"`
	KeyID       string `json:"key_id"`
}

type UpdateRequest struct {
	SchemaVersion int             `json:"schema_version"`
	RequestID     string          `json:"request_id"`
	Source        string          `json:"source"`
	TaskID        *string         `json:"task_id"`
	Force         bool            `json:"force"`
	TargetVersion string          `json:"target_version"`
	Manifest      any             `json:"manifest"`
	Artifact      ArtifactPayload `json:"artifact"`
}

type UpdateConfirmRequest struct {
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
}

type AppReadyRequest struct {
	SchemaVersion int       `json:"schema_version"`
	Version       string    `json:"version"`
	PID           int       `json:"pid"`
	ReadyAt       time.Time `json:"ready_at"`
}

type ServerHealthyRequest struct {
	SchemaVersion int       `json:"schema_version"`
	TaskID        string    `json:"task_id"`
	Version       string    `json:"version"`
	HeartbeatAt   time.Time `json:"heartbeat_at"`
}

type HealthStatus struct {
	Status string `json:"status"`
}

type StateResponseData struct {
	SessionID      string          `json:"session_id"`
	CurrentVersion string          `json:"current_version"`
	Health         HealthStatus    `json:"health"`
	Update         *UpdateSnapshot `json:"update"`
}

type IPCEvent struct {
	Sequence    int64     `json:"sequence"`
	Type        string    `json:"type"`
	CreatedAt   time.Time `json:"created_at"`
	OperationID string    `json:"operation_id"`
	Data        any       `json:"data"`
}

type EventsResponseData struct {
	SessionID   string     `json:"session_id"`
	CursorReset bool       `json:"cursor_reset"`
	Events      []IPCEvent `json:"events"`
}
