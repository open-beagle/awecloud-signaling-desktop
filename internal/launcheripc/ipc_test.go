package launcheripc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type mockCoordinator struct {
	connectCalled       bool
	appReadyCalled      bool
	serverHealthyCalled bool
}

func (m *mockCoordinator) HandleConnect(ctx context.Context, req *ConnectRequest) (*ConnectResponseData, error) {
	m.connectCalled = true
	return &ConnectResponseData{
		SessionID:       "session-123",
		LauncherPID:     100,
		ExpectedVersion: "1.1.1",
	}, nil
}

func (m *mockCoordinator) HandleUpdateApply(ctx context.Context, req *UpdateApplyRequest) (*UpdateAccepted, error) {
	return &UpdateAccepted{OperationID: "op-1", Phase: "waiting_for_exit", Accepted: true}, nil
}

func (m *mockCoordinator) HandleAppReady(ctx context.Context, req *AppReadyRequest) error {
	m.appReadyCalled = true
	return nil
}

func (m *mockCoordinator) HandleServerHealthy(ctx context.Context, req *ServerHealthyRequest) error {
	m.serverHealthyCalled = true
	return nil
}

func TestIPCServerClientHandshake(t *testing.T) {
	endpoint, err := NewEndpoint()
	require.NoError(t, err)
	token := "secret-token-12345"

	coord := &mockCoordinator{}
	server := NewServer(endpoint, token, ExpectedPeerUID(), coord)
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

	// 4. One-shot update handoff
	accepted, err := client.ApplyUpdate(ctx, &UpdateApplyRequest{RequestID: "request-1", TargetVersion: "1.1.1", Artifact: ArtifactPayload{ID: "artifact-1", SHA256: "abc"}})
	require.NoError(t, err)
	require.True(t, accepted.Accepted)
	require.Equal(t, "waiting_for_exit", accepted.Phase)
}

func TestIPCRejectsBrowserOrigin(t *testing.T) {
	coord := &mockCoordinator{}
	server := NewServer("unused", "token-123", ExpectedPeerUID(), coord)
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
	server := NewServer("unused", "valid-token", ExpectedPeerUID(), coord)
	handler := server.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/session/connect", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}
