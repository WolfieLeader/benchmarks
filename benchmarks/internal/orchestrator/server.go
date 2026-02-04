package orchestrator

import (
	"context"
	"fmt"
	"time"

	"benchmark-client/internal/cli"
	"benchmark-client/internal/client"
	"benchmark-client/internal/config"
	"benchmark-client/internal/container"
	"benchmark-client/internal/database"
	"benchmark-client/internal/summary"
)

func RunServerBenchmark(ctx context.Context, server *config.ResolvedServer, databases []string, network string) (*summary.ServerResult, []client.TimedResult, []client.TimedSequenceResult) {
	result := &summary.ServerResult{
		Name:      server.Name,
		ImageName: server.ImageName,
		Port:      server.Port,
		StartTime: time.Now(),
		Endpoints: make([]client.EndpointResult, 0),
	}

	if ctx.Err() != nil {
		result.SetError(ctx.Err())
		return result, nil, nil
	}

	options := container.StartOptions{
		Image:       server.ImageName,
		Port:        server.Port,
		HostPort:    8080,
		CpuLimit:    server.CpuLimit,
		MemoryLimit: server.MemoryLimit,
		Network:     network,
	}

	containerId, err := container.StartWithOptions(ctx, time.Minute, &options)
	if err != nil {
		result.SetError(fmt.Errorf("failed to start container: %w", err))
		return result, nil, nil
	}
	result.ContainerId = string(containerId)

	sampler := container.NewResourceSampler(string(containerId))
	sampler.Start(ctx)

	defer stopContainer(containerId) //nolint:contextcheck // intentionally uses fresh context for cleanup after cancellation

	serverUrl := fmt.Sprintf("http://localhost:%d", options.HostPort)
	if err = container.WaitToBeReady(ctx, 30*time.Second, serverUrl, databases); err != nil {
		stopSampler(sampler, result)
		result.SetError(fmt.Errorf("server did not become ready: %w", err))
		return result, nil, nil
	}

	cli.Successf("Ready at %s (container: %.12s)", serverUrl, containerId)

	if err = database.ResetAll(ctx, serverUrl, databases); err != nil {
		stopSampler(sampler, result)
		result.SetError(fmt.Errorf("failed to reset databases: %w", err))
		return result, nil, nil
	}
	cli.Infof("Reset all databases")

	if ctx.Err() != nil {
		stopSampler(sampler, result)
		result.SetError(ctx.Err())
		return result, nil, nil
	}

	result.StartTime = time.Now()

	suite := client.NewSuite(ctx, server)
	defer suite.Close()

	endpoints, err := suite.RunAll() //nolint:contextcheck // context is stored in Suite struct
	if err != nil {
		stopSampler(sampler, result)
		result.SetError(fmt.Errorf("benchmark failed: %w", err))
		return result, nil, nil
	}

	if ctx.Err() != nil {
		stopSampler(sampler, result)
		result.SetError(ctx.Err())
		return result, nil, nil
	}

	sequences := suite.RunSequences(options.HostPort) //nolint:contextcheck // context is stored in Suite struct

	timedResults := suite.GetTimedResults()
	timedSequences := suite.GetTimedSequences()

	stopSampler(sampler, result)

	result.Complete(endpoints)
	result.Sequences = sequences

	return result, timedResults, timedSequences
}

func stopContainer(containerId container.Id) {
	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Minute)
	defer stopCancel()
	if err := container.Stop(stopCtx, time.Minute, containerId); err != nil {
		cli.Warnf("Failed to stop container %s: %v", containerId, err)
	}
}

func stopSampler(sampler *container.ResourceSampler, result *summary.ServerResult) {
	if sampler != nil {
		stats := sampler.Stop()
		result.Resources = &stats
	}
}
