package metrics

import (
	"maps"
	"math/rand"
	"slices"
	"time"

	"benchmark-client/internal/client"
	"benchmark-client/internal/container"
)

const writeBatchSize = 5000

const (
	sourceEndpoint     = "endpoint"
	sourceSequence     = "sequence"
	sourceSequenceStep = "sequence_step"
	sourceServer       = "server"
	sourceDatabase     = "database"
)

// Column names shared across tables: the real timestamp plus the former Influx
// tags, now plain indexed columns (PLAN §9.1 decision 3).
const (
	colTime     = "time"
	colRunId    = "run_id"
	colServer   = "server"
	colSource   = "source"
	colDatabase = "database"
)

var requestEventColumns = []string{
	colTime, colRunId, colServer, "endpoint", "method", colSource, colDatabase,
	"server_offset_ms", "endpoint_offset_ms", "latency_ns",
}

// WriteEndpointLatencies streams the sampled raw endpoint events. Row time is
// the real wall clock of the request: server run start + the request's offset.
func (c *Client) WriteEndpointLatencies(runId, server string, start time.Time, results []client.TimedResult) {
	if c == nil {
		return
	}

	rows := make([][]any, 0, writeBatchSize)
	for _, r := range results {
		if c.ctx.Err() != nil {
			return
		}
		for _, l := range r.Latencies {
			if c.ctx.Err() != nil {
				return
			}
			if c.shouldSkipSample() {
				continue
			}
			rows = append(rows, []any{
				start.Add(l.ServerOffset), runId, server, r.Endpoint, r.Method, sourceEndpoint, "",
				l.ServerOffset.Milliseconds(), l.EndpointOffset.Milliseconds(), l.Duration.Nanoseconds(),
			})
			if len(rows) >= writeBatchSize {
				c.writeEventRowsAsync(rows)
				rows = rows[:0]
			}
		}
	}
	c.writeEventRowsAsync(rows)
}

// WriteSequenceLatencies streams the sampled raw sequence events: one
// source='sequence' row per full-sequence duration (endpoint = sequence id) and
// one source='sequence_step' row per step request.
func (c *Client) WriteSequenceLatencies(runId, server string, start time.Time, results []client.TimedSequenceResult) {
	if c == nil {
		return
	}

	rows := make([][]any, 0, writeBatchSize)
	for _, r := range results {
		if c.ctx.Err() != nil {
			return
		}
		for _, l := range r.Latencies {
			if c.ctx.Err() != nil {
				return
			}
			if c.shouldSkipSample() {
				continue
			}
			rows = append(rows, []any{
				start.Add(l.ServerOffset), runId, server, r.SequenceId, "", sourceSequence, r.Database,
				l.ServerOffset.Milliseconds(), l.EndpointOffset.Milliseconds(), l.Duration.Nanoseconds(),
			})
			if len(rows) >= writeBatchSize {
				c.writeEventRowsAsync(rows)
				rows = rows[:0]
			}
		}

		for _, stepName := range slices.Sorted(maps.Keys(r.StepStats)) {
			for _, l := range r.StepStats[stepName] {
				if c.ctx.Err() != nil {
					return
				}
				if c.shouldSkipSample() {
					continue
				}
				rows = append(rows, []any{
					start.Add(l.ServerOffset), runId, server, stepName, "", sourceSequenceStep, r.Database,
					l.ServerOffset.Milliseconds(), l.EndpointOffset.Milliseconds(), l.Duration.Nanoseconds(),
				})
				if len(rows) >= writeBatchSize {
					c.writeEventRowsAsync(rows)
					rows = rows[:0]
				}
			}
		}
	}
	c.writeEventRowsAsync(rows)
}

func (c *Client) shouldSkipSample() bool {
	if c.sampleRate > 0 && c.sampleRate < 1 && rand.Float64() >= c.sampleRate { //nolint:gosec // statistical sampling, not security
		c.pointsSampled.Add(1)
		return true
	}
	return false
}

var endpointStatColumns = []string{
	colTime, colRunId, colServer, "endpoint", "method", colSource, colDatabase,
	"count", "rps", "avg_ns", "p50_ns", "p95_ns", "p99_ns", "p999_ns", "min_ns", "max_ns", "success_rate",
	"target_rate", "offered_rate", "attempted", "dropped_iterations", "max_backlog",
	"schedule_lag_p50_ns", "schedule_lag_p99_ns", "schedule_lag_max_ns",
}

// WriteEndpointStats writes the exact per-endpoint aggregates, computed by the
// client from the full in-memory result set before any event sampling (§9.1
// decision 4) — never derived from the sampled request_events.
func (c *Client) WriteEndpointStats(runId, server string, results []client.EndpointResult) {
	if c == nil {
		return
	}

	now := time.Now()
	rows := make([][]any, 0, len(results))
	for i := range results {
		ep := &results[i]
		if ep.Stats == nil || ep.Stats.Count == 0 {
			continue
		}
		source := sourceEndpoint
		if ep.SequenceId != "" {
			source = sourceSequenceStep
		}
		var targetRate, offeredRate any
		var attempted, droppedIterations, maxBacklog any
		var lagP50, lagP99, lagMax any
		if ep.Open != nil {
			targetRate, offeredRate = ep.Open.TargetRate, ep.Open.OfferedRate
			attempted, droppedIterations = int64(ep.Open.Attempted), int64(ep.Open.DroppedIterations)
			maxBacklog = int64(ep.Open.MaxBacklog)
			lagP50 = ep.Open.ScheduleLagP50.Nanoseconds()
			lagP99 = ep.Open.ScheduleLagP99.Nanoseconds()
			lagMax = ep.Open.ScheduleLagMax.Nanoseconds()
		}
		rows = append(rows, []any{
			now, runId, server, ep.Name, ep.Method, source, ep.Database,
			int64(ep.Stats.Count),
			// rps counts completed requests (success + failure) per second of
			// wall time — pair with success_rate when dashboarding.
			ep.Stats.Rps,
			ep.Stats.Avg.Nanoseconds(), ep.Stats.P50.Nanoseconds(), ep.Stats.P95.Nanoseconds(),
			ep.Stats.P99.Nanoseconds(), ep.Stats.P999.Nanoseconds(),
			ep.Stats.Low.Nanoseconds(), ep.Stats.High.Nanoseconds(), ep.Stats.SuccessRate,
			targetRate, offeredRate, attempted, droppedIterations, maxBacklog, lagP50, lagP99, lagMax,
		})
	}
	_ = c.writeRows("endpoint_stats", endpointStatColumns, rows)
}

var sequenceStatColumns = []string{
	colTime, colRunId, colServer, "sequence_id", colDatabase,
	"total_runs", "successes", "failures", "success_rate",
	"avg_ns", "p50_ns", "p95_ns", "p99_ns",
}

// WriteSequenceStats writes the exact per-sequence aggregates (full-sequence
// durations), computed from the full in-memory result set before sampling.
func (c *Client) WriteSequenceStats(runId, server string, sequences []client.SequenceStats) {
	if c == nil {
		return
	}

	now := time.Now()
	rows := make([][]any, 0, len(sequences))
	for i := range sequences {
		seq := &sequences[i]
		if seq.TotalRuns == 0 {
			continue
		}
		rows = append(rows, []any{
			now, runId, server, seq.SequenceId, seq.Database,
			int64(seq.TotalRuns), int64(seq.Successes), int64(seq.Failures), seq.SuccessRate,
			seq.AvgDuration.Nanoseconds(), seq.P50Duration.Nanoseconds(),
			seq.P95Duration.Nanoseconds(), seq.P99Duration.Nanoseconds(),
		})
	}
	_ = c.writeRows("sequence_stats", sequenceStatColumns, rows)
}

var resourceSampleColumns = []string{
	colTime, colRunId, colServer, colSource, colDatabase,
	"memory_min_bytes", "memory_avg_bytes", "memory_max_bytes",
	"cpu_min_percent", "cpu_avg_percent", "cpu_max_percent", "samples",
}

func (c *Client) WriteResourceStats(runId, server string, stats *container.ResourceStats) {
	c.writeResourceRow(runId, server, sourceServer, "", stats)
}

// WriteDbResourceStats records a database container's resource usage during
// one server's run (PLAN §7.3): same resource_samples table, distinguished
// by source='database' + the database column.
func (c *Client) WriteDbResourceStats(runId, server, db string, stats *container.ResourceStats) {
	c.writeResourceRow(runId, server, sourceDatabase, db, stats)
}

func (c *Client) writeResourceRow(runId, server, source, db string, stats *container.ResourceStats) {
	if c == nil || stats == nil {
		return
	}

	_ = c.writeRows("resource_samples", resourceSampleColumns, [][]any{{
		time.Now(), runId, server, source, db,
		stats.Memory.MinBytes, stats.Memory.AvgBytes, stats.Memory.MaxBytes,
		stats.Cpu.MinPercent, stats.Cpu.AvgPercent, stats.Cpu.MaxPercent,
		int64(stats.Samples),
	}})
}
