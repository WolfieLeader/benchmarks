package orchestrator

import (
	"bufio"
	"context"
	"os"
	"time"

	"benchmark-client/internal/config"
	"benchmark-client/internal/database"
	"benchmark-client/internal/influx"
	"benchmark-client/internal/printer"
	"benchmark-client/internal/summary"
)

type Orchestrator struct {
	cfg       *config.Config
	servers   []*config.ResolvedServer
	compose   *database.ComposeManager
	writer    *summary.Writer
	databases []string
	influx    *influx.Client
	runId     string
}

func New(cfg *config.Config, servers []*config.ResolvedServer, repoRoot, resultsDir string) *Orchestrator {
	return &Orchestrator{
		cfg:       cfg,
		servers:   servers,
		compose:   database.NewComposeManager(repoRoot),
		writer:    summary.NewWriter(&cfg.Benchmark, resultsDir),
		databases: cfg.Databases,
		runId:     influx.RunID(time.Now()),
	}
}

func (o *Orchestrator) Run(ctx context.Context) error {
	printer.Section("Infrastructure")

	grafanaStarted := false
	if o.cfg.Influx.Enabled {
		printer.Infof("Starting Grafana stack...")
		if err := o.compose.StartGrafana(ctx); err != nil {
			return err
		}
		grafanaStarted = true
		printer.Successf("Grafana stack started")

		o.influx = influx.NewClient(ctx, influx.Config{
			Enabled: o.cfg.Influx.Enabled,
			URL:     o.cfg.Influx.URL,
			Org:     o.cfg.Influx.Org,
			Bucket:  o.cfg.Influx.Bucket,
			Token:   o.cfg.Influx.Token,
		})
	}

	printer.Infof("Starting database stack...")
	if err := o.compose.StartDatabases(ctx); err != nil {
		if grafanaStarted {
			o.stopGrafana(context.Background()) //nolint:contextcheck // cleanup must run even if ctx is canceled
		}
		return err
	}
	printer.Successf("Database stack started")

	printer.Infof("Waiting for databases to be healthy...")
	if err := o.compose.WaitHealthy(ctx, 2*time.Minute); err != nil {
		if grafanaStarted {
			o.stopGrafana(context.Background()) //nolint:contextcheck // cleanup must run even if ctx is canceled
		}
		return err
	}
	printer.Successf("All databases ready")

	interrupted := false
	cooldown := o.cfg.Benchmark.CooldownDuration
	for i, server := range o.servers {
		if ctx.Err() != nil {
			printer.Warnf("Interrupted, stopping...")
			interrupted = true
			break
		}

		printer.ServerHeader(server.Name)

		result, timedResults, timedFlows := RunServerBenchmark(ctx, server, o.databases, o.compose.NetworkName())

		summary.PrintServerSummary(result)
		path, err := o.writer.ExportServerResult(result)
		if err == nil {
			printer.Infof("Exported: %s", path)
		} else {
			printer.Failf("Failed to export %s results: %v", server.Name, err)
		}

		if o.influx != nil {
			o.influx.WriteEndpointLatencies(o.runId, server.Name, timedResults)
			o.influx.WriteFlowLatencies(o.runId, server.Name, timedFlows)
			if result.Capacity != nil {
				o.influx.WriteCapacityResult(o.runId, server.Name, result.Capacity)
			}
			if result.Resources != nil {
				o.influx.WriteResourceStats(o.runId, server.Name, result.Resources)
			}
			o.influx.Flush()
			printer.Infof("Exported metrics to InfluxDB (run: %s)", o.runId)
		}

		result.Endpoints = nil

		if ctx.Err() != nil {
			printer.Warnf("Interrupted, stopping...")
			interrupted = true
			break
		}

		if cooldown > 0 && i < len(o.servers)-1 {
			select {
			case <-ctx.Done():
				printer.Warnf("Interrupted, stopping...")
				interrupted = true
			case <-time.After(cooldown):
			}
			if interrupted {
				break
			}
		}
	}

	// Stop databases early - they're no longer needed after server benchmarks complete
	o.stopDatabases(context.Background()) //nolint:contextcheck // cleanup must run even if ctx is canceled

	metaResults, servers, path, err := o.writer.ExportMetaResults()
	if err != nil {
		if grafanaStarted {
			o.stopGrafana(context.Background()) //nolint:contextcheck // cleanup must run even if ctx is canceled
		}
		return err
	}
	printer.Infof("Meta results: %s", path)
	summary.PrintFinalSummary(metaResults, servers)

	if o.influx != nil {
		o.influx.Flush()
		o.influx.Close()
	}

	if grafanaStarted && !interrupted {
		o.waitForUserThenStopGrafana(ctx)
	} else if grafanaStarted {
		o.stopGrafana(context.Background()) //nolint:contextcheck // cleanup must run even if ctx is canceled
	}

	return nil
}

func (o *Orchestrator) waitForUserThenStopGrafana(ctx context.Context) {
	printer.Blank()
	printer.Infof("Grafana is running at http://localhost:3000 (admin/benchmark)")
	printer.Infof("Press Enter or Ctrl+C to stop Grafana and exit...")

	done := make(chan struct{})
	go func() {
		reader := bufio.NewReader(os.Stdin)
		_, _ = reader.ReadString('\n')
		close(done)
	}()

	select {
	case <-ctx.Done():
	case <-done:
	}

	o.stopGrafana(context.Background()) //nolint:contextcheck // cleanup must run even if ctx is canceled
}

func (o *Orchestrator) stopDatabases(ctx context.Context) {
	printer.Infof("Stopping database stack...")
	if err := o.compose.StopDatabases(ctx); err != nil {
		printer.Warnf("Failed to stop databases: %v", err)
	}
}

func (o *Orchestrator) stopGrafana(ctx context.Context) {
	printer.Infof("Stopping Grafana stack...")
	if err := o.compose.StopGrafana(ctx); err != nil {
		printer.Warnf("Failed to stop Grafana: %v", err)
	}
}
