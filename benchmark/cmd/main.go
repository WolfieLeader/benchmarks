package main

import (
	"benchmark-client/internal/client"
	"benchmark-client/internal/config"
	"benchmark-client/internal/container"
	"context"
	"fmt"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	config := config.GetConfig()

	for name, cfg := range config.Servers {
		// Start container
		containerId, err := container.Start(ctx, time.Minute, cfg.ImageName)
		if err != nil {
			fmt.Print(err)
			continue
		}

		// Wait for server to be ready - if not stop
		err = container.WaitToBeReady(ctx, 30*time.Second, config.Url)
		if err != nil {
			fmt.Printf("- Server in container %s did not become ready: %v\n", containerId, err)
			if stopErr := container.Stop(ctx, time.Minute, containerId); stopErr != nil {
				fmt.Print(stopErr)
			}
			continue
		}

		// Run tests
		client := client.New(ctx, config.Url)

		stats, err := client.Benchmark()
		if err != nil {
			fmt.Printf("- Benchmark error: %v\n", err)
			continue
		}

		fmt.Printf("- %s Container(%s) latency stats(success: %.2f%%):\n", name, containerId, stats.SuccessRate*100)
		fmt.Printf("- Average: %v (H:%v, L:%v). \n\n", stats.Avg, stats.High, stats.Low)

		// Stop container
		if stopErr := container.Stop(ctx, time.Minute, containerId); stopErr != nil {
			fmt.Print(stopErr)
		}
		time.Sleep(1 * time.Second)
	}
}
