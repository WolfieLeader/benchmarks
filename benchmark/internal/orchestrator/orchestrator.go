package orchestrator

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
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
	cfg            *config.Config
	servers        []*config.ResolvedServer
	compose        *database.ComposeManager
	writer         *summary.Writer
	databases      []string
	dbContainers   map[string]string // database service -> container ID, for resource sampling
	influx         *influx.Client
	runId          string
	noMetrics      bool
	exportFailures []string
}

const cleanupTimeout = 30 * time.Second

func New(cfg *config.Config, servers []*config.ResolvedServer, repoRoot, resultsDir string, noMetrics bool) *Orchestrator {
	return &Orchestrator{
		cfg:       cfg,
		servers:   servers,
		compose:   database.NewComposeManager(repoRoot),
		writer:    summary.NewWriter(&cfg.Benchmark, resultsDir),
		databases: cfg.Databases,
		runId:     influx.RunId(time.Now()),
		noMetrics: noMetrics,
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

	if o.noMetrics {
		cli.Warnf("Metrics disabled (--no-metrics): results JSON is still written, no metrics exported")
	} else {
		client, err := influx.NewClient(ctx, o.cfg.Benchmark.SampleRatePct)
		if err != nil {
			o.cleanupGrafana() //nolint:contextcheck // cleanup uses fresh context
			return fmt.Errorf("metrics DB unreachable (pass --no-metrics to run without it): %w", err)
		}
		o.influx = client
		defer o.influx.Close()
	}

	cli.Infof("Starting database stack...")
	stack, err := o.compose.EnsureDatabases(ctx)
	if err != nil {
		o.cleanupGrafana() //nolint:contextcheck // cleanup uses fresh context
		return err
	}
	if stack.Owned {
		cli.Successf("Database stack started (project %s)", stack.Project)
	} else {
		cli.Successf("Reusing running database stack (project %s) — it will be left running", stack.Project)
	}

	cli.Infof("Waiting for databases to be healthy...")
	if healthErr := o.compose.WaitHealthy(ctx, 2*time.Minute, o.databases); healthErr != nil {
		o.cleanupStacks() //nolint:contextcheck // cleanup uses fresh context
		return healthErr
	}
	cli.Successf("All databases ready")

	dbContainers, err := o.compose.DatabaseContainers(ctx, o.databases)
	if err != nil {
		o.cleanupStacks() //nolint:contextcheck // cleanup uses fresh context
		return fmt.Errorf("failed to resolve database containers for resource sampling: %w", err)
	}
	o.dbContainers = dbContainers

	interrupted := o.runBenchmarkLoop(ctx)

	flushErr := o.finalizeMetrics() //nolint:contextcheck // uses stored context from Client

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

	// Surface dropped/failed exports as a non-zero exit AFTER results have printed.
	return o.runFailure(flushErr)
}

// finalizeMetrics drains outstanding async writes, records the run_meta row with
// the accounting counters, prints the accounting, and returns any flush failure.
func (o *Orchestrator) finalizeMetrics() error {
	if o.influx == nil {
		return nil
	}

	flushErr := o.influx.Wait()
	// A canceled run_meta write means the run was interrupted, not that metrics
	// were dropped — cancellation is excluded from the no-silent-drop failure.
	if err := o.influx.WriteRunMeta(o.runId, o.cfg.Benchmark.SampleRatePct); err != nil && flushErr == nil && !errors.Is(err, context.Canceled) {
		flushErr = err
	}

	acct := o.influx.Accounting()
	cli.Section("Metrics accounting")
	cli.KeyValue("Points written", strconv.FormatInt(acct.PointsWritten, 10))
	cli.KeyValue("Points dropped", strconv.FormatInt(acct.PointsDropped, 10))
	cli.KeyValue("Points sampled out", strconv.FormatInt(acct.PointsSampledOut, 10))

	return flushErr
}

// runFailure combines the no-silent-drop failure modes into a single run error so
// the process exits non-zero while the in-memory results still print.
func (o *Orchestrator) runFailure(flushErr error) error {
	var msgs []string
	if len(o.exportFailures) > 0 {
		msgs = append(msgs, "failed to export results for: "+strings.Join(o.exportFailures, ", "))
	}
	if flushErr != nil {
		msgs = append(msgs, flushErr.Error())
	}
	if len(msgs) == 0 {
		return nil
	}
	return errors.New(strings.Join(msgs, "; "))
}

func (o *Orchestrator) runBenchmarkLoop(ctx context.Context) (interrupted bool) {
	cooldown := o.cfg.Benchmark.ServerCooldown

	for i, server := range o.servers {
		if ctx.Err() != nil {
			cli.Warnf("Interrupted, stopping...")
			return true
		}

		cli.ServerHeader(server.Name)

		result, timedResults, timedSequences := RunServerBenchmark(ctx, server, o.databases, o.compose.NetworkName(), o.dbContainers)

		summary.PrintServerSummary(result)
		path, err := o.writer.ExportServerResult(result)
		if err == nil {
			cli.Infof("Exported: %s", path)
		} else {
			cli.Failf("Failed to export %s results: %v", server.Name, err)
			o.exportFailures = append(o.exportFailures, server.Name)
		}

		if o.influx != nil {
			o.influx.WriteEndpointLatencies(o.runId, server.Name, timedResults)   //nolint:contextcheck // uses stored context from Client
			o.influx.WriteSequenceLatencies(o.runId, server.Name, timedSequences) //nolint:contextcheck // uses stored context from Client
			o.influx.WriteEndpointStats(o.runId, server.Name, result.Results)     //nolint:contextcheck // uses stored context from Client
			if result.Resources != nil {
				o.influx.WriteResourceStats(o.runId, server.Name, result.Resources) //nolint:contextcheck // uses stored context from Client
			}
			for db, stats := range result.DbResources {
				o.influx.WriteDbResourceStats(o.runId, server.Name, db, stats) //nolint:contextcheck // uses stored context from Client
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
			if err := o.compose.WaitHealthy(ctx, 2*time.Minute, o.databases); err != nil {
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
	if stack := o.compose.Stack(); stack != nil && !stack.Owned {
		cli.Infof("Leaving adopted database stack running (project %s)", stack.Project)
		return
	}
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
