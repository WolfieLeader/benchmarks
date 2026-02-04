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

	o.influx = influx.NewClient(ctx, influx.Config{
		Url:        o.cfg.Influx.Url,
		Database:   o.cfg.Influx.Database,
		Token:      o.cfg.Influx.Token,
		SampleRate: o.cfg.Influx.SampleRatePct,
	})

	cli.Infof("Starting database stack...")
	if err := o.compose.StartDatabases(ctx); err != nil {
		o.stopGrafana(context.Background()) //nolint:contextcheck // cleanup must run even if ctx is canceled
		return err
	}
	cli.Successf("Database stack started")

	cli.Infof("Waiting for databases to be healthy...")
	if err := o.compose.WaitHealthy(ctx, 2*time.Minute); err != nil {
		o.stopGrafana(context.Background()) //nolint:contextcheck // cleanup must run even if ctx is canceled
		return err
	}
	cli.Successf("All databases ready")

	interrupted := false
	cooldown := o.cfg.Benchmark.ServerCooldown
	for i, server := range o.servers {
		if ctx.Err() != nil {
			cli.Warnf("Interrupted, stopping...")
			interrupted = true
			break
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
			if result.Resources != nil {
				o.influx.WriteResourceStats(o.runId, server.Name, result.Resources) //nolint:contextcheck // uses stored context from Client
			}
			cli.Infof("Exported metrics to InfluxDB (run: %s)", o.runId)
		}

		result.Endpoints = nil

		if ctx.Err() != nil {
			cli.Warnf("Interrupted, stopping...")
			interrupted = true
			break
		}

		if cooldown > 0 && i < len(o.servers)-1 {
			select {
			case <-ctx.Done():
				cli.Warnf("Interrupted, stopping...")
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
		o.stopGrafana(context.Background()) //nolint:contextcheck // cleanup must run even if ctx is canceled
		return err
	}
	cli.Infof("Meta results: %s", path)
	summary.PrintFinalSummary(metaResults, servers)

	if o.influx != nil {
		o.influx.Wait()
		o.influx.Close()
	}

	if !interrupted {
		o.waitForUserThenStopGrafana(ctx)
	} else {
		o.stopGrafana(context.Background()) //nolint:contextcheck // cleanup must run even if ctx is canceled
	}

	return nil
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

	o.stopGrafana(context.Background()) //nolint:contextcheck // cleanup must run even if ctx is canceled
}

func (o *Orchestrator) stopDatabases(ctx context.Context) {
	cli.Infof("Stopping database stack...")
	if err := o.compose.StopDatabases(ctx); err != nil {
		cli.Warnf("Failed to stop databases: %v", err)
	}
}

func (o *Orchestrator) stopGrafana(ctx context.Context) {
	cli.Infof("Stopping Grafana stack...")
	if err := o.compose.StopGrafana(ctx); err != nil {
		cli.Warnf("Failed to stop Grafana: %v", err)
	}
}

func (o *Orchestrator) checkImages(ctx context.Context) []string {
	imageNames := make([]string, len(o.servers))
	for i, server := range o.servers {
		imageNames[i] = server.ImageName
	}
	return container.CheckImages(ctx, imageNames)
}
