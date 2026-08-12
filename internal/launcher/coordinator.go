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
	mu            sync.Mutex
	eventCond     *sync.Cond
	events        []launcheripc.IPCEvent
	nextSeq       int64
	current       *CurrentInfo
	previous      *CurrentInfo
	task          *UpdateTaskState
	health        *HealthInfo
	appReadyCh    chan struct{}
	restartCh     chan struct{}
}

func NewCoordinator(paths *Paths, serverAddress string) (*Coordinator, error) {
	c := &Coordinator{
		paths:         paths,
		serverAddress: serverAddress,
		sessionID:     fmt.Sprintf("session-%d", time.Now().UnixNano()),
		launcherPID:   os.Getpid(),
		appReadyCh:    make(chan struct{}, 1),
		restartCh:     make(chan struct{}, 1),
	}
	c.eventCond = sync.NewCond(&c.mu)

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
		c.nextSeq = task.Sequence + 1
	} else {
		c.nextSeq = 1
	}

	var health HealthInfo
	if err := readStateJSON(c.paths.HealthFile, &health); err == nil {
		c.health = &health
	}
	return nil
}

func (c *Coordinator) HandleConnect(ctx context.Context, req *launcheripc.ConnectRequest) (*launcheripc.ConnectResponseData, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.currentAppPID = req.PID

	var updateSnap *launcheripc.UpdateSnapshot
	if c.task != nil {
		updateSnap = &launcheripc.UpdateSnapshot{
			OperationID: c.task.OperationID,
			Phase:       c.task.Phase,
			Progress:    c.task.Progress,
			Error:       c.task.Error,
		}
	}

	expectedVer := "1.0.0"
	if c.current != nil {
		expectedVer = c.current.Version
	}

	return &launcheripc.ConnectResponseData{
		SessionID:       c.sessionID,
		LauncherPID:     c.launcherPID,
		ExpectedVersion: expectedVer,
		Recovered:       false,
		NextEventSeq:    c.nextSeq,
		Update:          updateSnap,
	}, nil
}

func (c *Coordinator) HandleUpdateRequest(ctx context.Context, req *launcheripc.UpdateRequest) (*launcheripc.UpdateSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.task != nil && (c.task.Phase != "succeeded" && c.task.Phase != "failed" && c.task.Phase != "rolled_back") {
		if req.TaskID != nil && c.task.TaskID != nil && *req.TaskID == *c.task.TaskID {
			return &launcheripc.UpdateSnapshot{
				OperationID: c.task.OperationID,
				Phase:       c.task.Phase,
				Progress:    c.task.Progress,
				Error:       c.task.Error,
			}, nil
		}
		return nil, errors.New("update in progress")
	}

	if req.TargetVersion == "latest" || req.Artifact.SHA256 == "" {
		currentVersion, currentSHA := "", ""
		if c.current != nil {
			currentVersion, currentSHA = c.current.Version, c.current.Artifact.SHA256
		}
		manifest, err := FetchPublicManifest(ctx, c.serverAddress, currentVersion, currentSHA)
		if err != nil {
			return nil, err
		}
		if strings.EqualFold(currentSHA, manifest.Artifacts.App.SHA256) {
			return nil, errors.New("Desktop App is already current")
		}
		req.TargetVersion = manifest.Release.Version
		req.Artifact = *manifest.Artifacts.App
	}

	opID := req.RequestID
	if req.TaskID != nil && *req.TaskID != "" {
		opID = *req.TaskID
	}

	targetApp := c.paths.ArtifactAppName(req.TargetVersion, req.Artifact.SHA256)

	c.task = &UpdateTaskState{
		SchemaVersion: 1,
		OperationID:   opID,
		RequestID:     req.RequestID,
		TaskID:        req.TaskID,
		Source:        req.Source,
		TargetVersion: req.TargetVersion,
		TargetApp:     targetApp,
		Force:         req.Force,
		Phase:         "accepted",
		Progress:      0,
		Sequence:      c.nextSeq,
		Artifact:      req.Artifact,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	c.nextSeq++

	_ = atomicWriteJSON(c.paths.UpdateTaskFile, c.task)
	c.broadcastEventLocked("update_progress", map[string]any{"phase": "accepted", "progress": 0})

	go c.runBackgroundDownload(req.Artifact)

	return &launcheripc.UpdateSnapshot{
		OperationID: opID,
		Phase:       "accepted",
		Progress:    0,
	}, nil
}

func (c *Coordinator) runBackgroundDownload(art launcheripc.ArtifactPayload) {
	c.updatePhase("downloading", 10, nil)

	res, err := DownloadAndVerifyArtifact(context.Background(), c.paths.DownloadsDir, &art, func(p int) {
		c.updatePhase("downloading", p, nil)
	})

	if err != nil {
		c.updatePhase("failed", 0, &launcheripc.ErrorDetail{Code: launcheripc.ErrDownloadFailed, Message: err.Error()})
		return
	}

	c.updatePhase("verifying", 90, nil)

	appDst := filepath.Join(c.paths.VersionsDir, c.task.TargetApp)
	_ = os.Remove(appDst)
	if err := os.Rename(res.PartPath, appDst); err != nil {
		c.updatePhase("failed", 0, &launcheripc.ErrorDetail{Code: launcheripc.ErrInstallFailed, Message: err.Error()})
		return
	}
	_ = os.Chmod(appDst, 0755)

	c.updatePhase("staged", 100, nil)
}

func (c *Coordinator) updatePhase(phase string, progress int, errDetail *launcheripc.ErrorDetail) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.task == nil {
		return
	}

	c.task.Phase = phase
	c.task.Progress = progress
	c.task.Error = errDetail
	c.task.Sequence = c.nextSeq
	c.nextSeq++
	c.task.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	_ = atomicWriteJSON(c.paths.UpdateTaskFile, c.task)

	if phase == "failed" || phase == "succeeded" || phase == "rolled_back" {
		c.broadcastEventLocked("update_result", map[string]any{"result": phase, "error": errDetail})
	} else {
		c.broadcastEventLocked("update_progress", map[string]any{"phase": phase, "progress": progress})
	}
}

func (c *Coordinator) HandleUpdateConfirm(ctx context.Context, req *launcheripc.UpdateConfirmRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.task == nil || c.task.Phase != "staged" {
		return errors.New("update task is not in staged phase")
	}

	if req.OperationID != c.task.OperationID {
		return errors.New("update operation does not match staged task")
	}
	if c.current == nil {
		return errors.New("current Desktop App state is unavailable")
	}
	previous := *c.current
	next := CurrentInfo{SchemaVersion: 1, Version: c.task.TargetVersion, App: c.task.TargetApp, Artifact: c.task.Artifact, InstalledAt: time.Now().UTC().Format(time.RFC3339)}
	if err := atomicWriteJSON(c.paths.PreviousFile, &previous); err != nil {
		return err
	}
	if err := atomicWriteJSON(c.paths.CurrentFile, &next); err != nil {
		return err
	}
	c.previous, c.current = &previous, &next
	c.task.Phase = "restarting"
	_ = atomicWriteJSON(c.paths.UpdateTaskFile, c.task)
	select {
	case c.restartCh <- struct{}{}:
	default:
	}
	return nil
}

func (c *Coordinator) RestartRequested() <-chan struct{} { return c.restartCh }

func (c *Coordinator) Current() *CurrentInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current == nil {
		return nil
	}
	current := *c.current
	return &current
}

func (c *Coordinator) HandleAppReady(ctx context.Context, req *launcheripc.AppReadyRequest) error {
	select {
	case c.appReadyCh <- struct{}{}:
	default:
	}
	return nil
}

func (c *Coordinator) HandleServerHealthy(ctx context.Context, req *launcheripc.ServerHealthyRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.task != nil && (c.task.Phase == "restarting" || c.task.Phase == "locally_healthy") {
		c.task.Phase = "succeeded"
		c.task.Progress = 100
		c.task.Sequence = c.nextSeq
		c.nextSeq++
		_ = atomicWriteJSON(c.paths.UpdateTaskFile, c.task)
		c.broadcastEventLocked("update_result", map[string]any{"result": "succeeded", "error": nil})
	}
	return nil
}

func (c *Coordinator) GetState(ctx context.Context) (*launcheripc.StateResponseData, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	curVer := "1.0.0"
	if c.current != nil {
		curVer = c.current.Version
	}

	var updateSnap *launcheripc.UpdateSnapshot
	if c.task != nil {
		updateSnap = &launcheripc.UpdateSnapshot{
			OperationID: c.task.OperationID,
			Phase:       c.task.Phase,
			Progress:    c.task.Progress,
			Error:       c.task.Error,
		}
	}

	status := "healthy"
	if c.health != nil {
		status = c.health.Status
	}

	return &launcheripc.StateResponseData{
		SessionID:      c.sessionID,
		CurrentVersion: curVer,
		Health:         launcheripc.HealthStatus{Status: status},
		Update:         updateSnap,
	}, nil
}

func (c *Coordinator) GetEvents(ctx context.Context, afterSeq int64, waitSec int) (*launcheripc.EventsResponseData, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	resEvents := make([]launcheripc.IPCEvent, 0)
	for _, e := range c.events {
		if e.Sequence > afterSeq {
			resEvents = append(resEvents, e)
		}
	}

	if len(resEvents) > 0 || waitSec == 0 {
		return &launcheripc.EventsResponseData{
			SessionID:   c.sessionID,
			CursorReset: afterSeq > c.nextSeq,
			Events:      resEvents,
		}, nil
	}

	// Long poll wait up to waitSec
	c.eventCond.Wait()

	for _, e := range c.events {
		if e.Sequence > afterSeq {
			resEvents = append(resEvents, e)
		}
	}

	return &launcheripc.EventsResponseData{
		SessionID:   c.sessionID,
		CursorReset: false,
		Events:      resEvents,
	}, nil
}

func (c *Coordinator) broadcastEventLocked(eventType string, data any) {
	opID := ""
	if c.task != nil {
		opID = c.task.OperationID
	}

	evt := launcheripc.IPCEvent{
		Sequence:    c.nextSeq - 1,
		Type:        eventType,
		CreatedAt:   time.Now().UTC(),
		OperationID: opID,
		Data:        data,
	}
	c.events = append(c.events, evt)
	c.eventCond.Broadcast()
}
