package main

import (
	"benchmark-client/internal/client"
	"benchmark-client/internal/config"
	"benchmark-client/internal/container"
	"benchmark-client/internal/results"
	"context"
	"fmt"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, resolvedServers, err := config.Load("config.jsonc")
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		return
	}
	fmt.Println(cfg)

	collector := results.NewCollector(&cfg.Global)

	for _, server := range resolvedServers {
		if ctx.Err() != nil {
			fmt.Println("\nInterrupted, stopping...")
			break
		}

		fmt.Printf("\n== Server: %s ==\n", server.Name)

		result := runServerBenchmark(ctx, server)
		collector.AddServerResult(result)
		results.PrintServerSummary(result)

		if ctx.Err() != nil {
			fmt.Println("\nInterrupted, stopping...")
			break
		}
	}

	if err := collector.Export("results.json"); err != nil {
		fmt.Printf("Failed to export results: %v\n", err)
	} else {
		fmt.Println("\nResults exported to results.json")
	}

	results.PrintFinalSummary(collector.GetResults())
}

func runServerBenchmark(ctx context.Context, server *config.ResolvedServer) *results.ServerResult {
	result := &results.ServerResult{
		Name:        server.Name,
		ContainerID: "",
		ImageName:   server.ImageName,
		Port:        server.Port,
		StartTime:   time.Now(),
		Endpoints:   make([]client.EndpointResult, 0),
	}

	options := container.StartOptions{
		Image:       server.ImageName,
		Port:        server.Port,
		HostPort:    8080,
		CPULimit:    server.CPULimit,
		MemoryLimit: server.MemoryLimit,
	}

	containerId, err := container.StartWithOptions(ctx, time.Minute, options)
	if err != nil {
		result.SetError(fmt.Errorf("failed to start container: %w", err))
		return result
	}
	result.ContainerID = string(containerId)

	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Minute)
		defer stopCancel()
		if stopErr := container.Stop(stopCtx, time.Minute, containerId); stopErr != nil {
			fmt.Printf("Warning: failed to stop container %s: %v\n", containerId, stopErr)
		}
	}()

	serverURL := fmt.Sprintf("http://localhost:%d", options.HostPort)
	if err := container.WaitToBeReady(ctx, 30*time.Second, serverURL); err != nil {
		result.SetError(fmt.Errorf("server did not become ready: %w", err))
		return result
	}

	fmt.Printf("  Server ready at %s (container: %s)\n", serverURL, containerId)

	suite := client.NewSuite(ctx, server)
	defer suite.Close()

	endpoints, err := suite.RunAll()
	if err != nil {
		result.SetError(fmt.Errorf("benchmark failed: %w", err))
		return result
	}

	result.Complete(endpoints)

	return result
}
