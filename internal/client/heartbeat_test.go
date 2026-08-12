package client

import (
	"context"
	"io"
	"sync"
	"testing"

	pb "github.com/open-beagle/awecloud-signaling-desktop/pkg/proto"
	"google.golang.org/grpc/metadata"
)

type heartbeatTestStream struct {
	mu         sync.Mutex
	closeCalls int
}

func (s *heartbeatTestStream) Send(*pb.DesktopHeartbeatRequest) error { return io.EOF }
func (s *heartbeatTestStream) Recv() (*pb.DesktopHeartbeatResponse, error) {
	return nil, io.EOF
}
func (s *heartbeatTestStream) Header() (metadata.MD, error) { return nil, nil }
func (s *heartbeatTestStream) Trailer() metadata.MD         { return nil }
func (s *heartbeatTestStream) CloseSend() error {
	s.mu.Lock()
	s.closeCalls++
	s.mu.Unlock()
	return nil
}
func (s *heartbeatTestStream) Context() context.Context { return context.Background() }
func (s *heartbeatTestStream) SendMsg(any) error        { return nil }
func (s *heartbeatTestStream) RecvMsg(any) error        { return nil }

func TestInvalidateHeartbeatOnlyClaimsCurrentStreamOnce(t *testing.T) {
	client := NewDesktopClient("127.0.0.1:1")
	stream := &heartbeatTestStream{}
	stopCh := make(chan struct{})
	client.heartbeatStream = stream
	client.heartbeatStopCh = stopCh

	results := make(chan bool, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- client.invalidateHeartbeat(stream, stopCh)
		}()
	}
	wg.Wait()
	close(results)

	claimed := 0
	for result := range results {
		if result {
			claimed++
		}
	}
	if claimed != 1 {
		t.Fatalf("failed stream was claimed %d times, want 1", claimed)
	}
	select {
	case <-stopCh:
	default:
		t.Fatal("failed stream generation was not stopped")
	}
	stream.mu.Lock()
	closeCalls := stream.closeCalls
	stream.mu.Unlock()
	if closeCalls != 1 {
		t.Fatalf("CloseSend called %d times, want 1", closeCalls)
	}
}
