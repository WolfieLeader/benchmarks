package influx

import (
	"maps"
	"math/rand"
	"slices"
	"time"

	"benchmark-client/internal/client"
	"benchmark-client/internal/container"

	"github.com/InfluxCommunity/influxdb3-go/influxdb3"
)

const writeBatchSize = 5000

const (
	tagRunId          = "run_id"
	tagServer         = "server"
	tagEndpoint       = "endpoint"
	tagMethod         = "method"
	tagSource         = "source"
	tagDatabase       = "database"
	fieldServerOffset = "server_offset_ms"
	fieldLatencyNs    = "latency_ns"
)

func (c *Client) WriteEndpointLatencies(runId, server string, results []client.TimedResult) {
	if c == nil {
		return
	}

	baseTime := time.Now()
	points := make([]*influxdb3.Point, 0, writeBatchSize)
	pointIndex := 0
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
			points = append(points, influxdb3.NewPoint(
				"request_latency",
				map[string]string{
					tagRunId:    runId,
					tagServer:   server,
					tagEndpoint: r.Endpoint,
					tagMethod:   r.Method,
					tagSource:   "endpoint",
				},
				map[string]any{
					fieldServerOffset:    l.ServerOffset.Milliseconds(),
					"endpoint_offset_ms": l.EndpointOffset.Milliseconds(),
					fieldLatencyNs:       l.Duration.Nanoseconds(),
				},
				baseTime.Add(time.Duration(pointIndex)*time.Microsecond),
			))
			pointIndex++
			if len(points) >= writeBatchSize {
				c.writePointsAsync(points)
				points = points[:0]
			}
		}
	}
	c.writePointsAsync(points)
}

func (c *Client) WriteSequenceLatencies(runId, server string, results []client.TimedSequenceResult) {
	if c == nil {
		return
	}

	baseTime := time.Now()
	points := make([]*influxdb3.Point, 0, writeBatchSize)
	pointIndex := 0
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
			points = append(points, influxdb3.NewPoint(
				"sequence_latency",
				map[string]string{
					tagRunId:      runId,
					tagServer:     server,
					"sequence_id": r.SequenceId,
					tagDatabase:   r.Database,
				},
				map[string]any{
					fieldServerOffset:    l.ServerOffset.Milliseconds(),
					"sequence_offset_ms": l.EndpointOffset.Milliseconds(),
					"total_duration_ns":  l.Duration.Nanoseconds(),
				},
				baseTime.Add(time.Duration(pointIndex)*time.Microsecond),
			))
			pointIndex++
			if len(points) >= writeBatchSize {
				c.writePointsAsync(points)
				points = points[:0]
			}
		}

		for _, stepName := range slices.Sorted(maps.Keys(r.StepStats)) {
			latencies := r.StepStats[stepName]
			for _, l := range latencies {
				if c.ctx.Err() != nil {
					return
				}
				if c.shouldSkipSample() {
					continue
				}
				ts := baseTime.Add(time.Duration(pointIndex) * time.Microsecond)
				points = append(points, influxdb3.NewPoint(
					"sequence_step_latency",
					map[string]string{
						tagRunId:      runId,
						tagServer:     server,
						"sequence_id": r.SequenceId,
						tagDatabase:   r.Database,
						"step":        stepName,
					},
					map[string]any{
						fieldServerOffset: l.ServerOffset.Milliseconds(),
						fieldLatencyNs:    l.Duration.Nanoseconds(),
					},
					ts,
				))
				pointIndex++
				points = append(points, influxdb3.NewPoint(
					"request_latency",
					map[string]string{
						tagRunId:    runId,
						tagServer:   server,
						tagEndpoint: stepName,
						tagMethod:   "",
						tagSource:   "sequence_step",
						tagDatabase: r.Database,
					},
					map[string]any{
						fieldServerOffset:    l.ServerOffset.Milliseconds(),
						"endpoint_offset_ms": l.EndpointOffset.Milliseconds(),
						fieldLatencyNs:       l.Duration.Nanoseconds(),
					},
					ts,
				))
				pointIndex++
				if len(points) >= writeBatchSize {
					c.writePointsAsync(points)
					points = points[:0]
				}
			}
		}
	}
	c.writePointsAsync(points)
}

func (c *Client) shouldSkipSample() bool {
	return c.sampleRate > 0 && c.sampleRate < 1 && rand.Float64() >= c.sampleRate //nolint:gosec // statistical sampling, not security
}

func (c *Client) WriteEndpointStats(runId, server string, results []client.EndpointResult) {
	if c == nil {
		return
	}

	baseTime := time.Now()
	for i := range results {
		ep := &results[i]
		if ep.Stats == nil || ep.Stats.Count == 0 {
			continue
		}
		source := "endpoint"
		if ep.SequenceId != "" {
			source = "sequence_step"
		}
		tags := map[string]string{
			tagRunId:    runId,
			tagServer:   server,
			tagEndpoint: ep.Name,
			tagMethod:   ep.Method,
			tagSource:   source,
		}
		if ep.Database != "" {
			tags[tagDatabase] = ep.Database
		}
		fields := map[string]any{
			"count":        int64(ep.Stats.Count),
			"rps":          ep.Stats.Rps,
			"avg_ns":       ep.Stats.Avg.Nanoseconds(),
			"p50_ns":       ep.Stats.P50.Nanoseconds(),
			"p95_ns":       ep.Stats.P95.Nanoseconds(),
			"p99_ns":       ep.Stats.P99.Nanoseconds(),
			"p999_ns":      ep.Stats.P999.Nanoseconds(),
			"min_ns":       ep.Stats.Low.Nanoseconds(),
			"max_ns":       ep.Stats.High.Nanoseconds(),
			"success_rate": ep.Stats.SuccessRate,
		}
		if ep.Open != nil {
			fields["target_rate"] = ep.Open.TargetRate
			fields["offered_rate"] = ep.Open.OfferedRate
			fields["dropped_iterations"] = int64(ep.Open.DroppedIterations)
			fields["schedule_lag_p99"] = ep.Open.ScheduleLagP99.Nanoseconds()
		}
		c.WritePoint("endpoint_stats", tags, fields, baseTime.Add(time.Duration(i)*time.Microsecond))
	}
}

func (c *Client) WriteResourceStats(runId, server string, stats *container.ResourceStats) {
	if c == nil || stats == nil {
		return
	}

	c.WritePoint("resource_stats",
		map[string]string{
			tagRunId:  runId,
			tagServer: server,
		},
		map[string]any{
			"memory_min_bytes": stats.Memory.MinBytes,
			"memory_avg_bytes": stats.Memory.AvgBytes,
			"memory_max_bytes": stats.Memory.MaxBytes,
			"cpu_min_percent":  stats.Cpu.MinPercent,
			"cpu_avg_percent":  stats.Cpu.AvgPercent,
			"cpu_max_percent":  stats.Cpu.MaxPercent,
			"samples":          stats.Samples,
		},
		time.Now(),
	)
}

func (c *Client) WriteRunMeta(runId string, sampleRate float64) {
	if c == nil {
		return
	}

	c.WritePoint("run_meta",
		map[string]string{
			tagRunId: runId,
		},
		map[string]any{
			"sample_rate": sampleRate,
		},
		time.Now(),
	)
}
