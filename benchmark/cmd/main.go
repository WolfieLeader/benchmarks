package main

import (
	"benchmark-client/internal/container"
	"context"
	"fmt"
	"os/signal"
	"syscall"
	"time"
)

var serverImages = []string{
	"go-chi-image",
	"go-fiber-image",
	"go-gin-image",
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	for _, img := range serverImages {
		containerId, err := container.Start(ctx, time.Minute, img)
		if err != nil {
			fmt.Print(err)
			continue
		}

		if err := container.WaitToBeReady(); err != nil {
			fmt.Printf("- Server in container %s did not become ready: %v\n", containerId, err)
			stopErr := container.Stop(ctx, time.Minute, containerId)
			if stopErr != nil {
				fmt.Print(stopErr)
			}
			continue
		}

		stopErr := container.Stop(ctx, time.Minute, containerId)
		if stopErr != nil {
			fmt.Print(stopErr)
		}
		time.Sleep(1 * time.Second)
	}
}
