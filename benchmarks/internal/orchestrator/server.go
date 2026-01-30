package orchestrator

import (
	"context"
	"fmt"
	"time"

	"benchmark-client/internal/client"
	"benchmark-client/internal/config"
	"benchmark-client/internal/container"
	"benchmark-client/internal/database"
	"benchmark-client/internal/printer"
	"benchmark-client/internal/summary"
)

// RunServerBenchmark executes a complete benchmark for a single server.
func RunServerBenchmark(ctx context.Context, server *config.ResolvedServer, databases []string, network string) *summary.ServerResult {
	result := &summary.ServerResult{
		Name:      server.Name,
		ImageName: server.ImageName,
		Port:      server.Port,
		StartTime: time.Now(),
		Endpoints: make([]client.EndpointResult, 0),
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

	// Start resource sampling immediately after container creation
	var sampler *container.ResourceSampler
	if server.ResourcesEnabled {
		sampler = container.NewResourceSampler(string(containerId))
		sampler.Start(ctx)
	}

	defer stopContainer(containerId) //nolint:contextcheck // intentionally uses fresh context for cleanup after cancellation

	serverURL := fmt.Sprintf("http://localhost:%d", options.HostPort)
	if err = container.WaitToBeReady(ctx, 30*time.Second, serverURL); err != nil {
		stopSampler(sampler, result)
		result.SetError(fmt.Errorf("server did not become ready: %w", err))
		return result
	}

	printer.Successf("Ready at %s (container: %.12s)", serverURL, containerId)

	// Reset databases
	if err = database.ResetAll(ctx, serverURL, databases); err != nil {
		stopSampler(sampler, result)
		result.SetError(fmt.Errorf("failed to reset databases: %w", err))
		return result
	}
	printer.Infof("Reset all databases")

	// Mark benchmark start (after setup)
	result.StartTime = time.Now()

	// Run endpoint tests
	suite := client.NewSuite(ctx, server)
	defer suite.Close()

	endpoints, err := suite.RunAll() //nolint:contextcheck // context is stored in Suite struct
	if err != nil {
		stopSampler(sampler, result)
		result.SetError(fmt.Errorf("benchmark failed: %w", err))
		return result
	}

	// Run flow tests
	flows := suite.RunFlows(options.HostPort) //nolint:contextcheck // context is stored in Suite struct

	// Collect resource stats
	stopSampler(sampler, result)

	result.Complete(endpoints)
	result.Flows = flows

	// Run capacity test
	if server.Capacity.Enabled && ctx.Err() == nil {
		runCapacityTest(ctx, server, result)
	}

	return result
}

func stopContainer(containerId container.Id) {
	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Minute)
	defer stopCancel()
	if err := container.Stop(stopCtx, time.Minute, containerId); err != nil {
		printer.Warnf("Failed to stop container %s: %v", containerId, err)
	}
}

func stopSampler(sampler *container.ResourceSampler, result *summary.ServerResult) {
	if sampler != nil {
		stats := sampler.Stop()
		result.Resources = &stats
	}
}

func runCapacityTest(ctx context.Context, server *config.ResolvedServer, result *summary.ServerResult) {
	rootTC := findRootTestcase(server)
	if rootTC == nil {
		printer.Warnf("Skipping capacity test: no root endpoint testcase found")
		return
	}

	tester := client.NewCapacityTester(ctx, &server.Capacity, rootTC, server.Timeout)
	capResult, err := tester.Run() //nolint:contextcheck // context is stored in CapacityTester struct
	if err != nil {
		printer.Failf("Capacity test error: %v", err)
	} else {
		result.Capacity = capResult
	}
}

func findRootTestcase(server *config.ResolvedServer) *config.Testcase {
	for _, tc := range server.Testcases {
		if tc.EndpointName == "root" {
			return tc
		}
	}
	return nil
}
