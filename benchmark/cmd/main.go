package main

import (
	"benchmark-client/internal/client"
	"benchmark-client/internal/config"
	"benchmark-client/internal/container"
	"benchmark-client/internal/results"
	"context"
	"fmt"
	"os/signal"
	"slices"
	"syscall"
	"time"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Load configuration
	cfg, err := config.Load("config.json")
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		return
	}

	// Validate configuration
	if err := config.Validate(cfg); err != nil {
		fmt.Printf("Invalid configuration: %v\n", err)
		return
	}

	fmt.Printf("Loaded configuration with %d endpoints and %d servers\n",
		len(cfg.Endpoints), len(cfg.Servers))
	fmt.Printf("Global settings: workers=%d, iterations=%d, timeout=%s\n\n",
		cfg.Global.Workers, cfg.Global.Iterations, cfg.Global.Timeout)

	// Initialize results collector
	collector := results.NewCollector(&results.ConfigSummary{
		BaseURL:    cfg.Global.BaseURL,
		Timeout:    cfg.Global.Timeout,
		Workers:    cfg.Global.Workers,
		Iterations: cfg.Global.Iterations,
	})

	// Run benchmarks for each server
	for name, serverCfg := range cfg.Servers {
		// Check if context was cancelled (Ctrl+C)
		if ctx.Err() != nil {
			fmt.Println("\nInterrupted, stopping...")
			break
		}

		fmt.Printf("=== Benchmarking %s ===\n", name)

		result := runServerBenchmark(ctx, cfg, name, serverCfg)
		collector.AddServerResult(result)
		printServerSummary(name, result)

		// Check again after benchmark
		if ctx.Err() != nil {
			fmt.Println("\nInterrupted, stopping...")
			break
		}

		// Small delay between servers
		time.Sleep(1 * time.Second)
	}

	// Export results
	if err := collector.Export("results.json"); err != nil {
		fmt.Printf("Failed to export results: %v\n", err)
	} else {
		fmt.Println("\nResults exported to results.json")
	}

	// Print final summary
	printFinalSummary(collector.GetResults())
}

func runServerBenchmark(ctx context.Context, cfg *config.Config, name string, serverCfg config.ServerConfig) *results.ServerResult {
	// Apply server-specific overrides to global config
	resolvedCfg := config.ApplyServerOverrides(&cfg.Global, &serverCfg)

	// Create result object
	result := results.NewServerResult(name, "", serverCfg.ImageName, serverCfg.Port)

	// Set resource usage info
	if serverCfg.Container.CPULimit != "" || serverCfg.Container.MemoryLimit != "" {
		result.ResourceUsage = &results.ResourceUsage{
			CPULimit:    serverCfg.Container.CPULimit,
			MemoryLimit: serverCfg.Container.MemoryLimit,
		}
	}

	// Start container with options
	opts := container.StartOptions{
		Image:       serverCfg.ImageName,
		Port:        serverCfg.Port,
		HostPort:    8080, // Use fixed host port for now
		CPULimit:    serverCfg.Container.CPULimit,
		MemoryLimit: serverCfg.Container.MemoryLimit,
	}

	containerId, err := container.StartWithOptions(ctx, time.Minute, opts)
	if err != nil {
		result.SetError(fmt.Errorf("failed to start container: %w", err))
		return result
	}
	result.ContainerID = string(containerId)

	// Ensure container is stopped on return (use fresh context so it works even if main ctx is cancelled)
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Minute)
		defer stopCancel()
		if stopErr := container.Stop(stopCtx, time.Minute, containerId); stopErr != nil {
			fmt.Printf("Warning: failed to stop container %s: %v\n", containerId, stopErr)
		}
	}()

	// Wait for server to be ready
	serverURL := fmt.Sprintf("http://localhost:%d", opts.HostPort)
	if err := container.WaitToBeReady(ctx, 30*time.Second, serverURL); err != nil {
		result.SetError(fmt.Errorf("server did not become ready: %w", err))
		return result
	}

	fmt.Printf("  Server ready at %s (container: %s)\n", serverURL, containerId)

	// Create and run test suite
	suite := client.NewSuite(ctx, cfg, serverURL)
	endpoints, err := suite.RunAll(resolvedCfg.Workers, resolvedCfg.Iterations)
	if err != nil {
		result.SetError(fmt.Errorf("benchmark failed: %w", err))
		return result
	}

	// Complete the result
	result.Complete(endpoints, resolvedCfg.Iterations)

	return result
}

func printServerSummary(name string, result *results.ServerResult) {
	if result.Error != "" {
		fmt.Printf("  FAILED: %s\n\n", result.Error)
		return
	}

	fmt.Printf("  Duration: %s\n", result.Duration)

	if result.OverallStats != nil {
		stats := result.OverallStats
		fmt.Printf("  Endpoints tested: %d\n", stats.EndpointCount)
		fmt.Printf("  Total requests: %d (success: %.2f%%)\n",
			stats.TotalRequests, stats.SuccessRate*100)
		fmt.Printf("  Latency - Avg: %s, Min: %s, Max: %s\n",
			stats.AvgLatency, stats.MinLatency, stats.MaxLatency)
		fmt.Printf("  Latency - P50: %s, P95: %s, P99: %s\n",
			stats.P50Latency, stats.P95Latency, stats.P99Latency)
	}

	// Print per-endpoint summary with stats
	fmt.Println("  Endpoints:")
	for _, ep := range result.Endpoints {
		status := "OK"
		if ep.Error != "" {
			status = "FAILED"
		} else if ep.Stats != nil && ep.Stats.SuccessRate < 1.0 {
			status = fmt.Sprintf("%.0f%%", ep.Stats.SuccessRate*100)
		}
		fmt.Printf("    %s %s [%s]\n", ep.Method, ep.Path, status)
		if ep.Stats != nil {
			fmt.Printf("      Avg: %s | P50: %s | P95: %s | P99: %s | Min: %s | Max: %s\n",
				ep.Stats.Avg, ep.Stats.P50, ep.Stats.P95, ep.Stats.P99, ep.Stats.Low, ep.Stats.High)
		}
	}
	fmt.Println()
}

func printFinalSummary(results *results.BenchmarkResults) {
	fmt.Println("\n=== Benchmark Summary ===")
	fmt.Printf("Total servers: %d\n", results.Summary.TotalServers)
	fmt.Printf("Successful: %d\n", results.Summary.SuccessfulServers)
	fmt.Printf("Failed: %d\n", results.Summary.FailedServers)
	fmt.Printf("Total duration: %s\n", results.Summary.TotalDuration)

	// Collect successful servers for ranking
	type rankedServer struct {
		Name       string
		AvgLatency int64
		P50Latency int64
		P95Latency int64
		P99Latency int64
	}

	ranked := make([]rankedServer, 0)
	for _, s := range results.Servers {
		if s.Error == "" && s.OverallStats != nil {
			ranked = append(ranked, rankedServer{
				Name:       s.Name,
				AvgLatency: s.OverallStats.AvgLatencyNs,
				P50Latency: s.OverallStats.P50LatencyNs,
				P95Latency: s.OverallStats.P95LatencyNs,
				P99Latency: s.OverallStats.P99LatencyNs,
			})
		}
	}

	if len(ranked) > 0 {
		// Sort by average latency (fastest first)
		slices.SortFunc(ranked, func(a, b rankedServer) int {
			if a.AvgLatency < b.AvgLatency {
				return -1
			}
			if a.AvgLatency > b.AvgLatency {
				return 1
			}
			return 0
		})

		fmt.Println("\n=== Server Rankings (by avg latency) ===")
		for i, s := range ranked {
			fmt.Printf("  %d. %s - Avg: %s | P50: %s | P95: %s | P99: %s\n",
				i+1,
				s.Name,
				time.Duration(s.AvgLatency).String(),
				time.Duration(s.P50Latency).String(),
				time.Duration(s.P95Latency).String(),
				time.Duration(s.P99Latency).String(),
			)
		}
	}
}
