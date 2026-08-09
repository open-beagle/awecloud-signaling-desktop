package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/launcher"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/launcheripc"
)

func main() {
	fmt.Println("=== Beagle Signal Launcher Starting ===")

	paths, err := launcher.NewPaths("")
	if err != nil {
		log.Fatalf("Initialize paths failed: %v", err)
	}

	coord, err := launcher.NewCoordinator(paths)
	if err != nil {
		log.Fatalf("Initialize coordinator failed: %v", err)
	}

	endpoint := os.Getenv(launcheripc.EnvLauncherEndpoint)
	if endpoint == "" {
		endpoint = fmt.Sprintf("/tmp/beagle-signal-%d/l-%d.sock", os.Getuid(), os.Getpid())
	}

	token := os.Getenv(launcheripc.EnvLauncherToken)
	if token == "" {
		token = fmt.Sprintf("token-%d", os.Getpid())
	}

	ipcServer := launcheripc.NewServer(endpoint, token, uint32(os.Getuid()), coord)
	if err := ipcServer.Start(); err != nil {
		log.Fatalf("Start IPC Server failed: %v", err)
	}
	defer ipcServer.Stop()

	fmt.Printf("Launcher running on endpoint: %s\n", endpoint)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("Launcher shutting down cleanly.")
}
