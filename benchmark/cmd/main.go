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

	cfg, err := config.Load(ctx, "config.jsonc")
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		return
	}

	fmt.Println("=== Benchmark Configuration ===")
	fmt.Printf("- Base URL: %s\n", cfg.BaseURL)
	fmt.Printf("- Workers: %d\n", cfg.Workers)
	fmt.Printf("- Iterations: %d\n", cfg.Iterations)
	fmt.Printf("- Servers: %d\n", len(cfg.Servers))
	fmt.Printf("- Timeout: %s\n\n", cfg.Timeout)

	collector := results.NewCollector(&results.ConfigSummary{
		BaseURL:    cfg.BaseURL,
		Timeout:    cfg.Timeout.String(),
		Workers:    cfg.Workers,
		Iterations: cfg.Iterations,
	})

	for _, server := range cfg.Servers {
		if ctx.Err() != nil {
			fmt.Println("\nInterrupted, stopping...")
			break
		}

		fmt.Printf("=== Benchmarking %s ===\n", server.Name)

		result := runServerBenchmark(ctx, cfg, server)
		collector.AddServerResult(result)
		printServerSummary(server.Name, result)

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

	printFinalSummary(collector.GetResults())
}

func runServerBenchmark(ctx context.Context, cfg *config.ResolvedConfig, server *config.ResolvedServer) *results.ServerResult {
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
		CPULimit:    cfg.CPULimit,
		MemoryLimit: cfg.MemoryLimit,
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

	suite := client.NewSuite(ctx, cfg, server, serverURL)
	endpoints, err := suite.RunAll()
	if err != nil {
		result.SetError(fmt.Errorf("benchmark failed: %w", err))
		return result
	}

	result.Complete(endpoints, server.Iterations)

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
