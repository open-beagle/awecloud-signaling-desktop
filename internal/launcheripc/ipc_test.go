package launcheripc

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type mockCoordinator struct {
	connectCalled      bool
	appReadyCalled     bool
	serverHealthyCalled bool
}

func (m *mockCoordinator) HandleConnect(ctx context.Context, req *ConnectRequest) (*ConnectResponseData, error) {
	m.connectCalled = true
	return &ConnectResponseData{
		SessionID:       "session-123",
		LauncherPID:     100,
		ExpectedVersion: "1.1.1",
		NextEventSeq:    1,
	}, nil
}

func (m *mockCoordinator) HandleUpdateRequest(ctx context.Context, req *UpdateRequest) (*UpdateSnapshot, error) {
	return &UpdateSnapshot{OperationID: "op-1", Phase: "accepted", Progress: 0}, nil
}

func (m *mockCoordinator) HandleUpdateConfirm(ctx context.Context, req *UpdateConfirmRequest) error {
	return nil
}

func (m *mockCoordinator) HandleAppReady(ctx context.Context, req *AppReadyRequest) error {
	m.appReadyCalled = true
	return nil
}

func (m *mockCoordinator) HandleServerHealthy(ctx context.Context, req *ServerHealthyRequest) error {
	m.serverHealthyCalled = true
	return nil
}

func (m *mockCoordinator) GetState(ctx context.Context) (*StateResponseData, error) {
	return &StateResponseData{
		SessionID:      "session-123",
		CurrentVersion: "1.1.1",
		Health:         HealthStatus{Status: "healthy"},
	}, nil
}

func (m *mockCoordinator) GetEvents(ctx context.Context, afterSeq int64, waitSec int) (*EventsResponseData, error) {
	return &EventsResponseData{
		SessionID:   "session-123",
		CursorReset: false,
		Events: []IPCEvent{
			{Sequence: 1, Type: "update_progress", CreatedAt: time.Now().UTC(), OperationID: "op-1"},
		},
	}, nil
}

func TestIPCServerClientHandshake(t *testing.T) {
	tmpDir := t.TempDir()
	endpoint := filepath.Join(tmpDir, "test.sock")
	token := "secret-token-12345"

	coord := &mockCoordinator{}
	server := NewServer(endpoint, token, uint32(os.Getuid()), coord)
	require.NoError(t, server.Start())
	defer server.Stop()

	// Wait for listener to be active
	time.Sleep(50 * time.Millisecond)

	client := NewClient(endpoint, token)
	ctx := context.Background()

	// 1. Connect
	connResp, err := client.Connect(ctx, &ConnectRequest{
		PID:              101,
		Version:          "1.1.1",
		ProcessStartedAt: time.Now().UTC().Format(time.RFC3339),
	})
	require.NoError(t, err)
	require.Equal(t, "session-123", connResp.SessionID)
	require.True(t, coord.connectCalled)

	// 2. App Ready
	require.NoError(t, client.SendAppReady(ctx, "1.1.1", 101))
	require.True(t, coord.appReadyCalled)

	// 3. Server Healthy
	require.NoError(t, client.SendServerHealthy(ctx, "task-1", "1.1.1"))
	require.True(t, coord.serverHealthyCalled)

	// 4. Get State
	state, err := client.GetState(ctx)
	require.NoError(t, err)
	require.Equal(t, "1.1.1", state.CurrentVersion)
	require.Equal(t, "healthy", state.Health.Status)

	// 5. Get Events (long polling)
	events, err := client.GetEvents(ctx, 0, 1)
	require.NoError(t, err)
	require.Len(t, events.Events, 1)
	require.Equal(t, int64(1), events.Events[0].Sequence)
}

func TestIPCRejectsBrowserOrigin(t *testing.T) {
	coord := &mockCoordinator{}
	server := NewServer("unused", "token-123", uint32(os.Getuid()), coord)
	handler := server.browserCheckMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/session/connect", nil)
	req.Header.Set("Origin", "http://malicious.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestIPCRejectsUnauthorizedToken(t *testing.T) {
	coord := &mockCoordinator{}
	server := NewServer("unused", "valid-token", uint32(os.Getuid()), coord)
	handler := server.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/session/connect", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

var _ = fmt.Printf
