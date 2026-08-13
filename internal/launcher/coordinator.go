package launcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/launcheripc"
)

type Coordinator struct {
	paths         *Paths
	serverAddress string
	sessionID     string
	launcherPID   int
	currentAppPID int
	appReadyPID   int
	mu            sync.Mutex
	current       *CurrentInfo
	previous      *CurrentInfo
	task          *UpdateTaskState
	health        *HealthInfo
	acceptedCh    chan struct{}
	rollbackCh    chan struct{}
	healthRun     uint64
}

func NewCoordinator(paths *Paths, serverAddress string) (*Coordinator, error) {
	c := &Coordinator{
		paths:         paths,
		serverAddress: serverAddress,
		sessionID:     fmt.Sprintf("session-%d", time.Now().UnixNano()),
		launcherPID:   os.Getpid(),
		acceptedCh:    make(chan struct{}, 1),
		rollbackCh:    make(chan struct{}, 1),
	}
	if err := c.reloadState(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Coordinator) reloadState() error {
	var current CurrentInfo
	if err := readStateJSON(c.paths.CurrentFile, &current); err == nil {
		c.current = &current
	}
	var previous CurrentInfo
	if err := readStateJSON(c.paths.PreviousFile, &previous); err == nil {
		c.previous = &previous
	}
	var task UpdateTaskState
	if err := readStateJSON(c.paths.UpdateTaskFile, &task); err == nil {
		c.task = &task
	}
	var health HealthInfo
	if err := readStateJSON(c.paths.HealthFile, &health); err == nil {
		c.health = &health
	}
	return nil
}

func (c *Coordinator) HandleConnect(_ context.Context, req *launcheripc.ConnectRequest) (*launcheripc.ConnectResponseData, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if req.PID <= 0 {
		return nil, errors.New("Desktop App PID is invalid")
	}
	if c.current != nil && !sameVersion(req.Version, c.current.Version) {
		return nil, errors.New("Desktop App version does not match current version")
	}
	c.currentAppPID = req.PID
	expectedVersion := "1.0.0"
	if c.current != nil {
		expectedVersion = c.current.Version
	}
	return &launcheripc.ConnectResponseData{
		SessionID:       c.sessionID,
		LauncherPID:     c.launcherPID,
		ExpectedVersion: expectedVersion,
		Recovered:       false,
	}, nil
}

func (c *Coordinator) HandleUpdateApply(ctx context.Context, req *launcheripc.UpdateApplyRequest) (*launcheripc.UpdateAccepted, error) {
	c.mu.Lock()
	if c.task != nil && !terminalPhase(c.task.Phase) {
		if req.RequestID == c.task.RequestID {
			accepted := acceptedFromTask(c.task, true)
			c.mu.Unlock()
			return accepted, nil
		}
		c.mu.Unlock()
		return nil, errors.New("update in progress")
	}
	if c.current == nil || c.currentAppPID <= 0 {
		c.mu.Unlock()
		return nil, errors.New("current Desktop App session is unavailable")
	}
	current := *c.current
	c.mu.Unlock()

	manifest, err := FetchPublicManifest(ctx, c.serverAddress)
	if err != nil {
		return nil, err
	}
	manifest.Update = EvaluateDesktopUpdate(current.Version, current.CommitID, manifest)
	if !manifest.Update.Available || strings.EqualFold(current.Artifact.SHA256, manifest.Artifacts.App.SHA256) {
		return nil, errors.New("Desktop App is already current")
	}
	if req.TargetVersion != "" && req.TargetVersion != manifest.Release.Version {
		return nil, errors.New("requested Desktop version does not match public manifest")
	}
	if req.Artifact.ID != manifest.Artifacts.App.ID || !strings.EqualFold(req.Artifact.SHA256, manifest.Artifacts.App.SHA256) {
		return nil, errors.New("requested Desktop artifact does not match public manifest")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	task := &UpdateTaskState{
		SchemaVersion:    1,
		OperationID:      req.RequestID,
		RequestID:        req.RequestID,
		Source:           "manual",
		TargetVersion:    manifest.Release.Version,
		TargetCommitID:   manifest.Release.CommitID,
		TargetCommitTime: manifest.Release.PublishedAt,
		TargetApp:        c.paths.LogicalAppName(manifest.Release.Version),
		Force:            req.Force,
		Phase:            "waiting_for_exit",
		Artifact:         *manifest.Artifacts.App,
		ReleaseNotes:     manifest.Release.ReleaseNotes,
		Required:         manifest.Update.Required || req.Force,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := atomicWriteJSON(c.paths.UpdateTaskFile, task); err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.task = task
	c.mu.Unlock()
	select {
	case c.acceptedCh <- struct{}{}:
	default:
	}
	return acceptedFromTask(task, false), nil
}

func acceptedFromTask(task *UpdateTaskState, duplicate bool) *launcheripc.UpdateAccepted {
	return &launcheripc.UpdateAccepted{
		OperationID: task.OperationID,
		Phase:       task.Phase,
		Accepted:    true,
		Duplicate:   duplicate,
	}
}

func terminalPhase(phase string) bool {
	return phase == "succeeded" || phase == "failed" || phase == "rolled_back"
}

func (c *Coordinator) UpdateAccepted() <-chan struct{}    { return c.acceptedCh }
func (c *Coordinator) RollbackRequested() <-chan struct{} { return c.rollbackCh }

func (c *Coordinator) HasAcceptedUpdate() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.task != nil && c.task.Phase == "waiting_for_exit"
}

func (c *Coordinator) CancelAcceptedUpdate(detail *launcheripc.ErrorDetail) {
	c.setTaskFailure("failed", detail)
}

func (c *Coordinator) ExecuteAcceptedUpdate(ctx context.Context) error {
	c.mu.Lock()
	if c.task == nil || c.task.Phase != "waiting_for_exit" {
		c.mu.Unlock()
		return errors.New("no accepted Desktop update is waiting for exit")
	}
	artifact := c.task.Artifact
	targetApp := c.task.TargetApp
	targetVersion := c.task.TargetVersion
	currentBeforeInstall := *c.current
	c.mu.Unlock()

	c.setTaskPhase("downloading")
	result, err := DownloadAndVerifyArtifact(ctx, c.paths.DownloadsDir, &artifact, nil)
	if err != nil {
		c.setTaskFailure("failed", &launcheripc.ErrorDetail{Code: launcheripc.ErrDownloadFailed, Message: err.Error()})
		return err
	}
	c.setTaskPhase("verifying")
	installedApp, err := installVersionedArtifact(c.paths, result.PartPath, targetVersion, artifact.SHA256)
	if err != nil {
		c.restoreCurrentEntryAfterFailedInstall(&currentBeforeInstall, targetVersion)
		c.setTaskFailure("failed", &launcheripc.ErrorDetail{Code: launcheripc.ErrInstallFailed, Message: err.Error()})
		return err
	}
	if installedApp != targetApp {
		err := errors.New("installed Desktop App name does not match update task")
		c.restoreCurrentEntryAfterFailedInstall(&currentBeforeInstall, targetVersion)
		c.setTaskFailure("failed", &launcheripc.ErrorDetail{Code: launcheripc.ErrInstallFailed, Message: err.Error()})
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current == nil || c.task == nil {
		return errors.New("Desktop update state disappeared during installation")
	}
	previous := *c.current
	previous.App = c.paths.ArtifactAppName(previous.Version, previous.Artifact.SHA256)
	next := CurrentInfo{
		SchemaVersion: 1,
		Version:       c.task.TargetVersion,
		CommitID:      c.task.TargetCommitID,
		CommitTime:    c.task.TargetCommitTime,
		App:           c.task.TargetApp,
		Artifact:      c.task.Artifact,
		InstalledAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := atomicWriteJSON(c.paths.PreviousFile, &previous); err != nil {
		c.restoreCurrentEntryAfterFailedInstall(&currentBeforeInstall, targetVersion)
		return err
	}
	if err := atomicWriteJSON(c.paths.CurrentFile, &next); err != nil {
		c.restoreCurrentEntryAfterFailedInstall(&currentBeforeInstall, targetVersion)
		return err
	}
	c.task.Phase = "restarting"
	c.task.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := atomicWriteJSON(c.paths.UpdateTaskFile, c.task); err != nil {
		_ = atomicWriteJSON(c.paths.CurrentFile, &previous)
		c.restoreCurrentEntryAfterFailedInstall(&currentBeforeInstall, targetVersion)
		return err
	}
	c.previous, c.current = &previous, &next
	previousLogicalPath := filepath.Join(c.paths.VersionsDir, c.paths.LogicalAppName(previous.Version))
	if previousLogicalPath != filepath.Join(c.paths.VersionsDir, next.App) {
		_ = os.Remove(previousLogicalPath)
	}
	return nil
}

func (c *Coordinator) restoreCurrentEntryAfterFailedInstall(current *CurrentInfo, targetVersion string) {
	if current == nil || !sameVersion(current.Version, targetVersion) {
		return
	}
	archivePath := filepath.Join(c.paths.VersionsDir, c.paths.ArtifactAppName(current.Version, current.Artifact.SHA256))
	_ = copyFileAtomically(archivePath, filepath.Join(c.paths.VersionsDir, c.paths.LogicalAppName(current.Version)))
}

func (c *Coordinator) setTaskPhase(phase string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.task == nil {
		return
	}
	c.task.Phase = phase
	c.task.Error = nil
	c.task.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	_ = atomicWriteJSON(c.paths.UpdateTaskFile, c.task)
}

func (c *Coordinator) setTaskFailure(phase string, detail *launcheripc.ErrorDetail) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.task == nil {
		return
	}
	c.task.Phase = phase
	c.task.Error = detail
	c.task.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	_ = atomicWriteJSON(c.paths.UpdateTaskFile, c.task)
}

func (c *Coordinator) Current() *CurrentInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current == nil {
		return nil
	}
	current := *c.current
	return &current
}

func (c *Coordinator) OperationForApp(current *CurrentInfo) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if current == nil || c.task == nil || c.task.Phase != "restarting" || !sameVersion(current.Version, c.task.TargetVersion) {
		return ""
	}
	return c.task.OperationID
}

func (c *Coordinator) BeginAppRun(current *CurrentInfo, pid int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if current == nil || c.task == nil || c.task.Phase != "restarting" || !sameVersion(current.Version, c.task.TargetVersion) {
		return
	}
	c.healthRun++
	run := c.healthRun
	now := time.Now().UTC()
	deadline := now.Add(90 * time.Second)
	c.health = &HealthInfo{
		SchemaVersion: 1, OperationID: c.task.OperationID, TargetVersion: current.Version,
		TargetApp: current.App, PID: pid, Status: "observing",
		StartedAt: now.Format(time.RFC3339), StartupDeadlineAt: deadline.Format(time.RFC3339),
	}
	_ = atomicWriteJSON(c.paths.HealthFile, c.health)
	if c.appReadyPID == pid {
		c.markLocallyHealthyLocked()
	}
	go func() {
		timer := time.NewTimer(time.Until(deadline))
		defer timer.Stop()
		<-timer.C
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.healthRun == run && c.health != nil && c.health.Status == "observing" {
			c.rollbackLocked(&launcheripc.ErrorDetail{Code: launcheripc.ErrAppReadyTimeout, Message: "new Desktop App did not become healthy within 90 seconds"})
		}
	}()
}

func (c *Coordinator) RollbackAfterFailure(detail *launcheripc.ErrorDetail) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.task == nil || (c.task.Phase != "restarting" && c.task.Phase != "locally_healthy") || c.previous == nil {
		return false
	}
	c.rollbackLocked(detail)
	select {
	case <-c.rollbackCh:
	default:
	}
	return true
}

func (c *Coordinator) rollbackLocked(detail *launcheripc.ErrorDetail) {
	if c.previous == nil || c.task == nil {
		return
	}
	previous := *c.previous
	failedApp := c.current.App
	archivePath := filepath.Join(c.paths.VersionsDir, c.paths.ArtifactAppName(previous.Version, previous.Artifact.SHA256))
	previous.App = c.paths.LogicalAppName(previous.Version)
	if err := copyFileAtomically(archivePath, filepath.Join(c.paths.VersionsDir, previous.App)); err != nil {
		c.task.Phase = "failed"
		c.task.Error = &launcheripc.ErrorDetail{Code: launcheripc.ErrRollbackFailed, Message: err.Error()}
		_ = atomicWriteJSON(c.paths.UpdateTaskFile, c.task)
		return
	}
	if err := atomicWriteJSON(c.paths.CurrentFile, &previous); err != nil {
		c.task.Phase = "failed"
		c.task.Error = &launcheripc.ErrorDetail{Code: launcheripc.ErrRollbackFailed, Message: err.Error()}
		_ = atomicWriteJSON(c.paths.UpdateTaskFile, c.task)
		return
	}
	c.current = &previous
	if failedApp != previous.App {
		_ = os.Remove(filepath.Join(c.paths.VersionsDir, failedApp))
	}
	c.healthRun++
	now := time.Now().UTC().Format(time.RFC3339)
	c.health = &HealthInfo{SchemaVersion: 1, OperationID: c.task.OperationID, TargetVersion: c.task.TargetVersion, TargetApp: c.task.TargetApp, Status: "rolled_back", StartedAt: now, StartupDeadlineAt: now, Failure: detail}
	c.task.Phase = "rolled_back"
	c.task.Error = detail
	c.task.UpdatedAt = now
	_ = atomicWriteJSON(c.paths.HealthFile, c.health)
	_ = atomicWriteJSON(c.paths.UpdateTaskFile, c.task)
	select {
	case c.rollbackCh <- struct{}{}:
	default:
	}
}

func (c *Coordinator) HandleAppReady(_ context.Context, req *launcheripc.AppReadyRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current == nil || !sameVersion(req.Version, c.current.Version) || req.PID != c.currentAppPID {
		return errors.New("Desktop App ready identity does not match current process")
	}
	c.appReadyPID = req.PID
	if c.task != nil && c.task.Phase == "restarting" && c.health != nil {
		c.markLocallyHealthyLocked()
	}
	return nil
}

func (c *Coordinator) markLocallyHealthyLocked() {
	if c.task == nil || c.health == nil || c.task.Phase != "restarting" {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	c.health.AppReadyAt = &now
	c.task.Phase = "locally_healthy"
	c.task.UpdatedAt = now
	_ = atomicWriteJSON(c.paths.HealthFile, c.health)
	_ = atomicWriteJSON(c.paths.UpdateTaskFile, c.task)
}

func (c *Coordinator) HandleServerHealthy(_ context.Context, req *launcheripc.ServerHealthyRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.task == nil || c.task.Phase != "locally_healthy" {
		return nil
	}
	if req.OperationID != c.task.OperationID || !sameVersion(req.Version, c.task.TargetVersion) {
		return errors.New("healthy Desktop App identity does not match active update")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	c.task.Phase = "succeeded"
	c.task.UpdatedAt = now
	c.task.Error = nil
	_ = atomicWriteJSON(c.paths.UpdateTaskFile, c.task)
	if c.health != nil {
		c.health.Status = "healthy"
		c.health.ServerHealthyAt = &now
		_ = atomicWriteJSON(c.paths.HealthFile, c.health)
	}
	c.healthRun++
	return nil
}

func sameVersion(left, right string) bool {
	return strings.TrimPrefix(strings.TrimSpace(left), "v") == strings.TrimPrefix(strings.TrimSpace(right), "v")
}
