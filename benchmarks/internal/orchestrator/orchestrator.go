package orchestrator

import (
	"context"
	"path/filepath"
	"time"

	"benchmark-client/internal/config"
	"benchmark-client/internal/database"
	"benchmark-client/internal/printer"
	"benchmark-client/internal/summary"
)

// Orchestrator manages the benchmark lifecycle.
type Orchestrator struct {
	cfg       *config.ConfigV2
	servers   []*config.ResolvedServer
	compose   *database.ComposeManager
	writer    *summary.Writer
	databases []string
}

// New creates a new Orchestrator.
func New(cfg *config.ConfigV2, servers []*config.ResolvedServer, resultsDir string) *Orchestrator {
	return &Orchestrator{
		cfg:       cfg,
		servers:   servers,
		compose:   database.NewComposeManager(filepath.Join("..", "infra", "compose", "databases.yml")),
		writer:    summary.NewWriter(&cfg.Benchmark, resultsDir),
		databases: cfg.Databases,
	}
}

// Run executes the full benchmark suite.
func (o *Orchestrator) Run(ctx context.Context) error {
	// Start database stack
	printer.Section("Database Stack")
	printer.Infof("Starting databases...")
	if err := o.compose.Start(ctx); err != nil {
		return err
	}
	defer o.stopDatabases() //nolint:contextcheck // intentionally uses fresh context for cleanup after cancellation

	printer.Infof("Waiting for databases to be healthy...")
	if err := o.compose.WaitHealthy(ctx, 2*time.Minute); err != nil {
		return err
	}
	printer.Successf("All databases ready")

	// Run benchmarks for each server
	cooldown := o.cfg.Benchmark.CooldownDuration
	for i, server := range o.servers {
		if ctx.Err() != nil {
			printer.Warnf("Interrupted, stopping...")
			break
		}

		printer.ServerHeader(server.Name)

		result := RunServerBenchmark(ctx, server, o.databases, o.compose.NetworkName())

		summary.PrintServerSummary(result)
		path, err := o.writer.ExportServerResult(result)
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

		if cooldown > 0 && i < len(o.servers)-1 {
			select {
			case <-ctx.Done():
				printer.Warnf("Interrupted, stopping...")
				return ctx.Err()
			case <-time.After(cooldown):
			}
		}
	}

	// Export final results
	metaResults, servers, path, err := o.writer.ExportMetaResults()
	if err != nil {
		return err
	}
	printer.Infof("Meta results: %s", path)
	summary.PrintFinalSummary(metaResults, servers)

	return nil
}

func (o *Orchestrator) stopDatabases() {
	printer.Infof("Stopping databases...")
	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Minute)
	defer stopCancel()
	if err := o.compose.Stop(stopCtx); err != nil {
		printer.Warnf("Failed to stop databases: %v", err)
	}
}
