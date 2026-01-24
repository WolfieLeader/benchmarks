package main

import (
	"benchmark-client/internal/client"
	"benchmark-client/internal/config"
	"benchmark-client/internal/container"
	"benchmark-client/internal/summary"
	"context"
	"fmt"
	"os/signal"
	"path/filepath"
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

	resultsDir := filepath.Join("results", time.Now().UTC().Format("20060102-150405"))
	writer := summary.NewWriter(&cfg.Global, resultsDir)

	for _, server := range resolvedServers {
		if ctx.Err() != nil {
			fmt.Println("\nInterrupted, stopping...")
			break
		}

		fmt.Printf("\n== Server: %s ==\n", server.Name)

		result := runServerBenchmark(ctx, server)

		summary.PrintServerSummary(result)
		path, err := writer.ExportServerResult(result)
		if err == nil {
			fmt.Printf("Server results exported to %s\n", path)
		} else {
			fmt.Printf("Failed to export %s results: %v\n", server.Name, err)
		}
		result.Endpoints = nil

		if ctx.Err() != nil {
			fmt.Println("\nInterrupted, stopping...")
			break
		}
	}

	metaResults, servers, path, err := writer.ExportMetaResults()
	if err != nil {
		fmt.Printf("Failed to export meta results: %v\n", err)
		return
	}
	fmt.Printf("\nMeta results exported to %s\n", path)

	summary.PrintFinalSummary(metaResults, servers)
}

func runServerBenchmark(ctx context.Context, server *config.ResolvedServer) *summary.ServerResult {
	result := &summary.ServerResult{
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
