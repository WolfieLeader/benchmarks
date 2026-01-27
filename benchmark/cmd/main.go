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

	cfg, resolvedServers, err := config.Load("config.json")
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		return
	}
	fmt.Println(cfg)

	var cooldown time.Duration
	if cfg.Global.Cooldown != "" {
		cooldown, err = time.ParseDuration(cfg.Global.Cooldown)
		if err != nil {
			fmt.Printf("Invalid cooldown: %v\n", err)
			return
		}
	}

	resultsDir := filepath.Join("results", time.Now().UTC().Format("20060102-150405"))
	writer := summary.NewWriter(&cfg.Global, resultsDir)

	for i, server := range resolvedServers {
		if ctx.Err() != nil {
			fmt.Println("\nInterrupted, stopping...")
			break
		}

		fmt.Printf("\n== Server: %s ==\n", server.Name)

		result := runServerBenchmark(ctx, server)

		summary.PrintServerSummary(result)
		var path string
		path, err = writer.ExportServerResult(result)
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

		if cooldown > 0 && i < len(resolvedServers)-1 {
			select {
			case <-ctx.Done():
				fmt.Println("\nInterrupted, stopping...")
				return
			case <-time.After(cooldown):
			}
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

	// Start resource sampling immediately after container creation (before health check)
	// This captures samples during startup, health check, warmup, and benchmark
	var sampler *container.ResourceSampler
	if server.Resources.Enabled {
		sampler = container.NewResourceSampler(string(containerId))
		sampler.Start(ctx)
	}

	defer func() { //nolint:contextcheck // intentionally uses fresh context for cleanup after cancellation
		stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Minute)
		defer stopCancel()
		if stopErr := container.Stop(stopCtx, time.Minute, containerId); stopErr != nil {
			fmt.Printf("Warning: failed to stop container %s: %v\n", containerId, stopErr)
		}
	}()

	serverURL := fmt.Sprintf("http://localhost:%d", options.HostPort)
	if err = container.WaitToBeReady(ctx, 30*time.Second, serverURL); err != nil {
		// Stop sampler on early exit
		if sampler != nil {
			sampler.Stop()
		}
		result.SetError(fmt.Errorf("server did not become ready: %w", err))
		return result
	}

	fmt.Printf("  Server ready at %s (container: %s)\n", serverURL, containerId)

	// Benchmark duration should reflect warmup + measured requests, not container startup
	result.StartTime = time.Now()

	suite := client.NewSuite(ctx, server)
	defer suite.Close()

	endpoints, err := suite.RunAll() //nolint:contextcheck // context is stored in Suite struct
	if err != nil {
		// Stop sampler even on error
		if sampler != nil {
			stats := sampler.Stop()
			result.Resources = &stats
		}
		result.SetError(fmt.Errorf("benchmark failed: %w", err))
		return result
	}

	// Stop resource sampling and collect stats
	if sampler != nil {
		stats := sampler.Stop()
		result.Resources = &stats
	}

	result.Complete(endpoints)

	// Run capacity test if enabled and not skipped
	if server.Capacity.Enabled && ctx.Err() == nil {
		rootTC := findRootTestcase(server)
		if rootTC != nil {
			fmt.Println("  Running capacity test...")
			tester := client.NewCapacityTester(ctx, &server.Capacity, rootTC, server.Timeout)
			capResult, err := tester.Run() //nolint:contextcheck // context is stored in CapacityTester struct
			if err != nil {
				fmt.Printf("  Capacity test error: %v\n", err)
			} else {
				result.Capacity = capResult
			}
		} else {
			fmt.Println("  Skipping capacity test: no root endpoint testcase found")
		}
	}

	return result
}

func findRootTestcase(server *config.ResolvedServer) *config.Testcase {
	for _, tc := range server.Testcases {
		if tc.EndpointName == "root" {
			return tc
		}
	}
	return nil
}
