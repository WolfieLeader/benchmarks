package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"benchmark-client/internal/cli"
	"benchmark-client/internal/client"
	"benchmark-client/internal/config"
	"benchmark-client/internal/container"
	"benchmark-client/internal/database"
	"benchmark-client/internal/printer"
	"benchmark-client/internal/summary"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, resolvedServers, err := config.LoadV2(config.DefaultConfigFile)
	if err != nil {
		printer.Failf("Failed to load configuration: %v", err)
		return
	}

	// Get runtime options from CLI flags or interactive prompt
	opts, err := getRuntimeOptions(config.GetServerNames(resolvedServers))
	if err != nil {
		printer.Failf("Failed to get options: %v", err)
		return
	}

	// Apply runtime options and filter servers
	var invalidServers []string
	resolvedServers, invalidServers = config.ApplyRuntimeOptionsV2(cfg, resolvedServers, opts)
	if len(invalidServers) > 0 {
		printer.Warnf("Unknown servers ignored: %s", strings.Join(invalidServers, ", "))
	}
	if len(resolvedServers) == 0 {
		printer.Failf("No valid servers selected")
		return
	}

	cfg.Print()

	// Cooldown is pre-parsed in v2 config
	cooldown := cfg.Benchmark.CooldownDuration

	// Start database compose stack
	compose := database.NewComposeManager(filepath.Join("..", "infra", "compose", "databases.yml"))
	printer.Section("Database Stack")
	printer.Infof("Starting databases...")
	if err = compose.Start(ctx); err != nil {
		printer.Failf("Failed to start databases: %v", err)
		return
	}
	defer func() {
		printer.Infof("Stopping databases...")
		stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Minute)
		defer stopCancel()
		if stopErr := compose.Stop(stopCtx); stopErr != nil {
			printer.Warnf("Failed to stop databases: %v", stopErr)
		}
	}()

	printer.Infof("Waiting for databases to be healthy...")
	if err = compose.WaitHealthy(ctx, 2*time.Minute); err != nil {
		printer.Failf("Databases did not become healthy: %v", err)
		return
	}
	printer.Successf("All databases ready")

	resultsDir := filepath.Join("..", "results", time.Now().UTC().Format("20060102-150405"))
	writer := summary.NewWriter(&cfg.Benchmark, resultsDir)

	for i, server := range resolvedServers {
		if ctx.Err() != nil {
			printer.Warnf("Interrupted, stopping...")
			break
		}

		printer.ServerHeader(server.Name)

		result := runServerBenchmark(ctx, server, cfg.Databases, compose.NetworkName())

		summary.PrintServerSummary(result)
		var path string
		path, err = writer.ExportServerResult(result)
		if err == nil {
			printer.Infof("Exported: %s", path)
		} else {
			printer.Failf("Failed to export %s results: %v", server.Name, err)
		}
		result.Endpoints = nil

		if ctx.Err() != nil {
			printer.Warnf("Interrupted, stopping...")
			break
		}

		if cooldown > 0 && i < len(resolvedServers)-1 {
			select {
			case <-ctx.Done():
				printer.Warnf("Interrupted, stopping...")
				return
			case <-time.After(cooldown):
			}
		}
	}

	metaResults, servers, path, err := writer.ExportMetaResults()
	if err != nil {
		printer.Failf("Failed to export meta results: %v", err)
		return
	}
	printer.Infof("Meta results: %s", path)

	summary.PrintFinalSummary(metaResults, servers)
}

func runServerBenchmark(ctx context.Context, server *config.ResolvedServer, databases []string, network string) *summary.ServerResult {
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
		Network:     network,
	}

	containerId, err := container.StartWithOptions(ctx, time.Minute, &options)
	if err != nil {
		result.SetError(fmt.Errorf("failed to start container: %w", err))
		return result
	}
	result.ContainerID = string(containerId)

	// Start resource sampling immediately after container creation (before health check)
	// This captures samples during startup, health check, warmup, and benchmark
	var sampler *container.ResourceSampler
	if server.ResourcesEnabled {
		sampler = container.NewResourceSampler(string(containerId))
		sampler.Start(ctx)
	}

	defer func() { //nolint:contextcheck // intentionally uses fresh context for cleanup after cancellation
		stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Minute)
		defer stopCancel()
		if stopErr := container.Stop(stopCtx, time.Minute, containerId); stopErr != nil {
			printer.Warnf("Failed to stop container %s: %v", containerId, stopErr)
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

	printer.Successf("Ready at %s (container: %.12s)", serverURL, containerId)

	// Reset all databases before running tests
	if err = database.ResetAll(ctx, serverURL, databases); err != nil {
		if sampler != nil {
			sampler.Stop()
		}
		result.SetError(fmt.Errorf("failed to reset databases: %w", err))
		return result
	}
	printer.Infof("Reset all databases")

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

	// Run flow-based tests if any flows are configured
	flows := suite.RunFlows(options.HostPort) //nolint:contextcheck // context is stored in Suite struct

	// Stop resource sampling and collect stats
	if sampler != nil {
		stats := sampler.Stop()
		result.Resources = &stats
	}

	result.Complete(endpoints)
	result.Flows = flows

	// Run capacity test if enabled and not skipped
	if server.Capacity.Enabled && ctx.Err() == nil {
		rootTC := findRootTestcase(server)
		if rootTC != nil {
			tester := client.NewCapacityTester(ctx, &server.Capacity, rootTC, server.Timeout)
			capResult, err := tester.Run() //nolint:contextcheck // context is stored in CapacityTester struct
			if err != nil {
				printer.Failf("Capacity test error: %v", err)
			} else {
				result.Capacity = capResult
			}
		} else {
			printer.Warnf("Skipping capacity test: no root endpoint testcase found")
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

func getRuntimeOptions(availableServers []string) (*config.RuntimeOptions, error) {
	// Try to parse CLI flags first
	cliOpts, err := cli.ParseFlags(os.Args[1:])
	if err != nil {
		if errors.Is(err, cli.ErrHelp) {
			os.Exit(0)
		}
		return nil, err
	}

	if cliOpts != nil {
		// Non-interactive mode: use CLI flags
		return &config.RuntimeOptions{
			Warmup:    cliOpts.Warmup,
			Resources: cliOpts.Resources,
			Capacity:  cliOpts.Capacity,
			Servers:   cliOpts.Servers,
		}, nil
	}

	// Interactive mode: show prompt
	cli.PrintBanner()
	opts, err := cli.PromptOptions(availableServers)
	if err != nil {
		return nil, err
	}

	cli.PrintSummary(opts, len(availableServers))

	return &config.RuntimeOptions{
		Warmup:    opts.Warmup,
		Resources: opts.Resources,
		Capacity:  opts.Capacity,
		Servers:   opts.Servers,
	}, nil
}
