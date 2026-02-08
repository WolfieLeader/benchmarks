package orchestrator

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"benchmark-client/internal/cli"
	"benchmark-client/internal/config"
	"benchmark-client/internal/container"
	"benchmark-client/internal/database"
	"benchmark-client/internal/influx"
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

const cleanupTimeout = 30 * time.Second

func New(cfg *config.Config, servers []*config.ResolvedServer, repoRoot, resultsDir string) *Orchestrator {
	return &Orchestrator{
		cfg:       cfg,
		servers:   servers,
		compose:   database.NewComposeManager(repoRoot),
		writer:    summary.NewWriter(&cfg.Benchmark, resultsDir),
		databases: cfg.Databases,
		runId:     influx.RunId(time.Now()),
	}
}

func (o *Orchestrator) Run(ctx context.Context) error {
	if missing := o.checkImages(ctx); len(missing) > 0 {
		return fmt.Errorf("missing Docker images: %s\nRun 'just images' to build them", strings.Join(missing, ", "))
	}

	cli.Section("Infrastructure")

	cli.Infof("Starting Grafana stack...")
	if err := o.compose.StartGrafana(ctx); err != nil {
		return err
	}
	cli.Successf("Grafana stack started")

	o.influx = influx.NewClient(ctx, o.cfg.Benchmark.SampleRatePct)
	if o.influx != nil {
		defer func() {
			o.influx.Wait()
			o.influx.Close()
		}()
		o.influx.WriteRunMeta(o.runId, o.cfg.Benchmark.SampleRatePct) //nolint:contextcheck // uses stored context from Client
	}

	cli.Infof("Starting database stack...")
	if err := o.compose.StartDatabases(ctx); err != nil {
		o.cleanupGrafana() //nolint:contextcheck // cleanup uses fresh context
		return err
	}
	cli.Successf("Database stack started")

	cli.Infof("Waiting for databases to be healthy...")
	if err := o.compose.WaitHealthy(ctx, 2*time.Minute); err != nil {
		o.cleanupStacks() //nolint:contextcheck // cleanup uses fresh context
		return err
	}
	cli.Successf("All databases ready")

	interrupted := o.runBenchmarkLoop(ctx)

	if o.influx != nil {
		o.influx.Wait()
	}

	o.cleanupDatabases() //nolint:contextcheck // cleanup uses fresh context

	metaResults, servers, path, err := o.writer.ExportMetaResults()
	if err != nil {
		o.cleanupGrafana() //nolint:contextcheck // cleanup uses fresh context
		return err
	}
	cli.Infof("Meta results: %s", path)
	summary.PrintFinalSummary(metaResults, servers)

	if !interrupted {
		o.waitForUserThenStopGrafana(ctx)
	} else {
		o.cleanupGrafana() //nolint:contextcheck // cleanup uses fresh context
	}

	return nil
}

func (o *Orchestrator) runBenchmarkLoop(ctx context.Context) (interrupted bool) {
	cooldown := o.cfg.Benchmark.ServerCooldown

	for i, server := range o.servers {
		if ctx.Err() != nil {
			cli.Warnf("Interrupted, stopping...")
			return true
		}

		cli.ServerHeader(server.Name)

		result, timedResults, timedSequences := RunServerBenchmark(ctx, server, o.databases, o.compose.NetworkName())

		summary.PrintServerSummary(result)
		path, err := o.writer.ExportServerResult(result)
		if err == nil {
			cli.Infof("Exported: %s", path)
		} else {
			cli.Failf("Failed to export %s results: %v", server.Name, err)
		}

		if o.influx != nil {
			o.influx.WriteEndpointLatencies(o.runId, server.Name, timedResults)   //nolint:contextcheck // uses stored context from Client
			o.influx.WriteSequenceLatencies(o.runId, server.Name, timedSequences) //nolint:contextcheck // uses stored context from Client
			o.influx.WriteEndpointStats(o.runId, server.Name, result.Results)     //nolint:contextcheck // uses stored context from Client
			if result.Resources != nil {
				o.influx.WriteResourceStats(o.runId, server.Name, result.Resources) //nolint:contextcheck // uses stored context from Client
			}
			cli.Infof("Exported metrics to InfluxDB (run: %s)", o.runId)
		}

		result.Results = nil

		if ctx.Err() != nil {
			cli.Warnf("Interrupted, stopping...")
			return true
		}

		if cooldown > 0 && i < len(o.servers)-1 {
			select {
			case <-ctx.Done():
				cli.Warnf("Interrupted, stopping...")
				return true
			case <-time.After(cooldown):
			}
		}

		if i < len(o.servers)-1 {
			cli.Infof("Verifying databases are healthy...")
			if err := o.compose.WaitHealthy(ctx, 2*time.Minute); err != nil {
				cli.Failf("Databases did not recover: %v", err)
				break
			}
			cli.Successf("All databases ready")
		}
	}

	return false
}

func (o *Orchestrator) waitForUserThenStopGrafana(ctx context.Context) {
	cli.Blank()
	cli.Infof("Grafana is running at http://localhost:3000 (admin/123456)")
	cli.Infof("Press Enter or Ctrl+C to stop Grafana and databases and exit...")

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

	o.cleanupGrafana() //nolint:contextcheck // cleanup uses fresh context
}

func (o *Orchestrator) cleanupDatabases() {
	cli.Infof("Stopping database stack...")
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	if err := o.compose.StopDatabases(ctx); err != nil {
		cli.Warnf("Failed to stop databases: %v", err)
	}
}

func (o *Orchestrator) cleanupGrafana() {
	cli.Infof("Stopping Grafana stack...")
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	if err := o.compose.StopGrafana(ctx); err != nil {
		cli.Warnf("Failed to stop Grafana: %v", err)
	}
}

func (o *Orchestrator) cleanupStacks() {
	o.cleanupDatabases()
	o.cleanupGrafana()
}

func (o *Orchestrator) checkImages(ctx context.Context) []string {
	imageNames := make([]string, len(o.servers))
	for i, server := range o.servers {
		imageNames[i] = server.ImageName
	}
	return container.CheckImages(ctx, imageNames)
}
