package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/launcheripc"
)

type acceptedUpdateCoordinator struct{}

func (acceptedUpdateCoordinator) HandleConnect(context.Context, *launcheripc.ConnectRequest) (*launcheripc.ConnectResponseData, error) {
	return &launcheripc.ConnectResponseData{}, nil
}

type rejectedUpdateCoordinator struct{ acceptedUpdateCoordinator }

func (rejectedUpdateCoordinator) HandleUpdateApply(context.Context, *launcheripc.UpdateApplyRequest) (*launcheripc.UpdateAccepted, error) {
	return nil, errors.New("update rejected")
}

func (acceptedUpdateCoordinator) HandleUpdateApply(_ context.Context, req *launcheripc.UpdateApplyRequest) (*launcheripc.UpdateAccepted, error) {
	return &launcheripc.UpdateAccepted{OperationID: req.RequestID, Phase: "waiting_for_exit", Accepted: true}, nil
}

func TestApplyDesktopUpdateStaysRunningWhenLauncherRejects(t *testing.T) {
	endpoint, err := launcheripc.NewEndpoint()
	require.NoError(t, err)
	token := "desktop-update-rejected-token"
	server := launcheripc.NewServer(endpoint, token, launcheripc.ExpectedPeerUID(), rejectedUpdateCoordinator{})
	require.NoError(t, server.Start())
	t.Cleanup(func() { _ = server.Stop() })
	t.Setenv(launcheripc.EnvLauncherEndpoint, endpoint)
	t.Setenv(launcheripc.EnvLauncherToken, token)

	exited := make(chan struct{}, 1)
	application := New()
	application.updateExit = func() { exited <- struct{}{} }
	_, err = application.ApplyDesktopUpdate(&launcheripc.UpdateApplyRequest{RequestID: "request-rejected"})
	require.ErrorContains(t, err, "update rejected")

	select {
	case <-exited:
		t.Fatal("Desktop started its exit path after Launcher rejected the update")
	case <-time.After(300 * time.Millisecond):
	}
}

func (acceptedUpdateCoordinator) HandleAppReady(context.Context, *launcheripc.AppReadyRequest) error {
	return nil
}

func (acceptedUpdateCoordinator) HandleServerHealthy(context.Context, *launcheripc.ServerHealthyRequest) error {
	return nil
}

func TestApplyDesktopUpdateExitsOnlyAfterLauncherAccepts(t *testing.T) {
	endpoint, err := launcheripc.NewEndpoint()
	require.NoError(t, err)
	token := "desktop-update-test-token"
	server := launcheripc.NewServer(endpoint, token, launcheripc.ExpectedPeerUID(), acceptedUpdateCoordinator{})
	require.NoError(t, server.Start())
	t.Cleanup(func() { _ = server.Stop() })
	t.Setenv(launcheripc.EnvLauncherEndpoint, endpoint)
	t.Setenv(launcheripc.EnvLauncherToken, token)

	exited := make(chan struct{}, 1)
	application := New()
	application.updateExit = func() { exited <- struct{}{} }
	accepted, err := application.ApplyDesktopUpdate(&launcheripc.UpdateApplyRequest{RequestID: "request-accepted"})
	require.NoError(t, err)
	require.True(t, accepted.Accepted)
	require.Equal(t, "waiting_for_exit", accepted.Phase)

	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("Desktop did not start its exit path after Launcher accepted the update")
	}
}
