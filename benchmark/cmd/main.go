package main

import (
	"benchmark-client/internal/client"
	"benchmark-client/internal/container"
	"context"
	"fmt"
	"os/signal"
	"syscall"
	"time"
)

const serverUrl = "http://localhost:8080"

var serverImages = []string{
	"go-chi-image",
	"go-fiber-image",
	"go-gin-image",
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	for _, img := range serverImages {
		// Start container
		containerId, err := container.Start(ctx, time.Minute, img)
		if err != nil {
			fmt.Print(err)
			continue
		}

		// Wait for server to be ready - if not stop
		err = container.WaitToBeReady(ctx, 30*time.Second, serverUrl)
		if err != nil {
			fmt.Printf("- Server in container %s did not become ready: %v\n", containerId, err)
			if stopErr := container.Stop(ctx, time.Minute, containerId); stopErr != nil {
				fmt.Print(stopErr)
			}
			continue
		}

		// Run tests
		client := client.New(ctx, serverUrl)
		fmt.Printf("Running benchmarks against server in container %s (%s)\n", containerId, img)
		client.RunBenchmarks()

		// Stop container
		if stopErr := container.Stop(ctx, time.Minute, containerId); stopErr != nil {
			fmt.Print(stopErr)
		}
		time.Sleep(1 * time.Second)
	}
}
