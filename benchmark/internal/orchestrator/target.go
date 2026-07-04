package orchestrator

import (
	"context"
	"fmt"
	"time"

	"benchmark-client/internal/cli"
	"benchmark-client/internal/client"
	"benchmark-client/internal/config"
	"benchmark-client/internal/database"
	"benchmark-client/internal/summary"
)

// RunTarget benchmarks a single externally-managed server (--target): the
// caller owns the server's lifecycle, so no containers, no compose stacks, no
// resource sampling, and no metrics DB — the suite runs against baseUrl and
// the result is exported as JSON only. Used by the oha calibration gate
// (PLAN §7.6) and for ad-hoc runs against an already-running server.
func RunTarget(ctx context.Context, cfg *config.Config, server *config.ResolvedServer, baseUrl, resultsDir string) error {
	writer := summary.NewWriter(&cfg.Benchmark, resultsDir)

	cli.ServerHeader(server.Name)
	cli.Infof("Benchmarking external target %s", baseUrl)

	result := &summary.ServerResult{
		Name:      server.Name,
		StartTime: time.Now(),
		Results:   make([]client.EndpointResult, 0),
	}

	if len(cfg.Databases) > 0 {
		if err := database.ResetAll(ctx, baseUrl, cfg.Databases); err != nil {
			return fmt.Errorf("failed to reset databases: %w", err)
		}
		cli.Infof("Reset all databases")
	}

	suiteOut, runErr := runSuite(ctx, server, baseUrl)
	if runErr != nil {
		result.SetError(runErr)
	} else {
		result.Complete(suiteOut.allResults())
		result.Sequences = suiteOut.sequences
	}

	summary.PrintServerSummary(result)
	path, err := writer.ExportServerResult(result)
	if err != nil {
		return fmt.Errorf("failed to export %s results: %w", server.Name, err)
	}
	cli.Infof("Exported: %s", path)

	return runErr
}
