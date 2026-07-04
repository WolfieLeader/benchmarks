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

	suiteOut, err := runSuite(ctx, server, serverUrl)
	stopSampler(sampler, result)
	if err != nil {
		result.SetError(err)
		return result, nil, nil
	}

	result.Complete(suiteOut.allResults())
	result.Sequences = suiteOut.sequences

	return result, suiteOut.timedResults, suiteOut.timedSequences
}

// suiteOutput carries everything a suite run produces; shared by the container
// path (RunServerBenchmark) and the external-target path (RunTarget).
type suiteOutput struct {
	endpoints      []client.EndpointResult
	sequences      []client.SequenceStats
	timedResults   []client.TimedResult
	timedSequences []client.TimedSequenceResult
}

func (s *suiteOutput) allResults() []client.EndpointResult {
	all := s.endpoints
	all = append(all, client.SequenceStepsToResults(s.sequences)...)
	return all
}

// runSuite drives the endpoint suite and sequences against serverUrl with a
// progress spinner. It owns no container or sampler state — callers do.
func runSuite(ctx context.Context, server *config.ResolvedServer, serverUrl string) (*suiteOutput, error) {
	endpointCount := countUniqueEndpoints(server.Testcases)
	sequenceCount := len(server.Sequences)

	progress := cli.NewProgressSpinner()
	progress.Start(endpointCount, sequenceCount)
	defer progress.Stop()

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
		return nil, fmt.Errorf("benchmark failed: %w", err)
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	sequences := suite.RunSequences() //nolint:contextcheck // context is stored in Suite struct

	return &suiteOutput{
		endpoints:      endpoints,
		sequences:      sequences,
		timedResults:   suite.GetTimedResults(),
		timedSequences: suite.GetTimedSequences(),
	}, nil
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
