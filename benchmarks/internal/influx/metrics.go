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
					"run_id":   runId,
					"server":   server,
					"endpoint": r.Endpoint,
					"method":   r.Method,
					"source":   "endpoint",
				},
				map[string]any{
					"server_offset_ms":   l.ServerOffset.Milliseconds(),
					"endpoint_offset_ms": l.EndpointOffset.Milliseconds(),
					"latency_ns":         l.Duration.Nanoseconds(),
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
					"run_id":      runId,
					"server":      server,
					"sequence_id": r.SequenceId,
					"database":    r.Database,
				},
				map[string]any{
					"server_offset_ms":   l.ServerOffset.Milliseconds(),
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
						"run_id":      runId,
						"server":      server,
						"sequence_id": r.SequenceId,
						"database":    r.Database,
						"step":        stepName,
					},
					map[string]any{
						"server_offset_ms": l.ServerOffset.Milliseconds(),
						"latency_ns":       l.Duration.Nanoseconds(),
					},
					ts,
				))
				pointIndex++
				points = append(points, influxdb3.NewPoint(
					"request_latency",
					map[string]string{
						"run_id":   runId,
						"server":   server,
						"endpoint": stepName,
						"method":   "",
						"source":   "sequence_step",
						"database": r.Database,
					},
					map[string]any{
						"server_offset_ms":   l.ServerOffset.Milliseconds(),
						"endpoint_offset_ms": l.EndpointOffset.Milliseconds(),
						"latency_ns":         l.Duration.Nanoseconds(),
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
			"run_id":   runId,
			"server":   server,
			"endpoint": ep.Name,
			"method":   ep.Method,
			"source":   source,
		}
		if ep.Database != "" {
			tags["database"] = ep.Database
		}
		c.WritePoint("endpoint_stats", tags, map[string]any{
			"count":        int64(ep.Stats.Count),
			"avg_ns":       ep.Stats.Avg.Nanoseconds(),
			"p50_ns":       ep.Stats.P50.Nanoseconds(),
			"p95_ns":       ep.Stats.P95.Nanoseconds(),
			"p99_ns":       ep.Stats.P99.Nanoseconds(),
			"min_ns":       ep.Stats.Low.Nanoseconds(),
			"max_ns":       ep.Stats.High.Nanoseconds(),
			"success_rate": ep.Stats.SuccessRate,
		}, baseTime.Add(time.Duration(i)*time.Microsecond))
	}
}

func (c *Client) WriteResourceStats(runId, server string, stats *container.ResourceStats) {
	if c == nil || stats == nil {
		return
	}

	c.WritePoint("resource_stats",
		map[string]string{
			"run_id": runId,
			"server": server,
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
			"run_id": runId,
		},
		map[string]any{
			"sample_rate": sampleRate,
		},
		time.Now(),
	)
}
