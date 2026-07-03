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
		Results:   make([]client.EndpointResult, 0),
	}

	if ctx.Err() != nil {
		result.SetError(ctx.Err())
		return result, nil, nil
	}

	// testcontainers starts the container, joins the DB network, applies limits,
	// waits for /health + each /db/<db>/health, and maps a dynamic host port.
	srv, err := container.Start(ctx, &container.StartOptions{
		Image:          server.ImageName,
		ContainerPort:  server.Port,
		CpuLimit:       server.CpuLimit,
		MemoryLimit:    server.MemoryLimit,
		Network:        network,
		Databases:      databases,
		StartupTimeout: 60 * time.Second,
	})
	if err != nil {
		result.SetError(fmt.Errorf("failed to start container: %w", err))
		return result, nil, nil
	}
	result.ContainerId = srv.ID

	sampler := container.NewResourceSampler(srv.ID)

	defer stopContainer(srv) //nolint:contextcheck // intentionally uses fresh context for cleanup after cancellation

	serverUrl := srv.BaseURL
	cli.Successf("Ready at %s (container: %.12s)", serverUrl, srv.ID)

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

	sampler.Start(ctx)
	result.StartTime = time.Now()

	endpointCount := countUniqueEndpoints(server.Testcases)
	sequenceCount := len(server.Sequences)

	progress := cli.NewProgressSpinner()
	progress.Start(endpointCount, sequenceCount)

	suite := client.NewSuite(ctx, server, serverUrl, &client.ProgressCallbacks{
		OnEndpoint: func(method, path string, done int) {
			progress.UpdateEndpoint(method, path, done)
		},
		OnSequence: func(seqName string, done int) {
			progress.UpdateSequence(seqName, done)
		},
	})
	defer suite.Close()

	endpoints, err := suite.RunAll() //nolint:contextcheck // context is stored in Suite struct
	if err != nil {
		progress.Stop()
		stopSampler(sampler, result)
		result.SetError(fmt.Errorf("benchmark failed: %w", err))
		return result, nil, nil
	}

	if ctx.Err() != nil {
		progress.Stop()
		stopSampler(sampler, result)
		result.SetError(ctx.Err())
		return result, nil, nil
	}

	sequences := suite.RunSequences() //nolint:contextcheck // context is stored in Suite struct

	progress.Stop()

	timedResults := suite.GetTimedResults()
	timedSequences := suite.GetTimedSequences()

	stopSampler(sampler, result)

	allResults := endpoints
	allResults = append(allResults, client.SequenceStepsToResults(sequences)...)
	result.Complete(allResults)
	result.Sequences = sequences

	return result, timedResults, timedSequences
}

func countUniqueEndpoints(testcases []*config.Testcase) int {
	seen := make(map[string]struct{})
	for _, tc := range testcases {
		seen[tc.EndpointName] = struct{}{}
	}
	return len(seen)
}

func stopContainer(srv *container.Server) {
	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Minute)
	defer stopCancel()
	if err := srv.Stop(stopCtx); err != nil {
		cli.Warnf("Failed to stop container %.12s: %v", srv.ID, err)
	}
}

func stopSampler(sampler *container.ResourceSampler, result *summary.ServerResult) {
	if sampler != nil {
		stats := sampler.Stop()
		result.Resources = &stats
	}
}
