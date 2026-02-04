package influx

import (
	"math/rand"
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
		if c.ctx != nil && c.ctx.Err() != nil {
			return
		}
		for _, l := range r.Latencies {
			if c.ctx != nil && c.ctx.Err() != nil {
				return
			}
			if c.sampleRate > 0 && c.sampleRate < 1 {
				if rand.Float64() >= c.sampleRate { //nolint:gosec // statistical sampling, not security
					continue
				}
			}
			points = append(points, influxdb3.NewPoint(
				"request_latency",
				map[string]string{
					"run_id":   runId,
					"server":   server,
					"endpoint": r.Endpoint,
					"method":   r.Method,
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
		if c.ctx != nil && c.ctx.Err() != nil {
			return
		}
		for _, l := range r.Latencies {
			if c.ctx != nil && c.ctx.Err() != nil {
				return
			}
			if c.sampleRate > 0 && c.sampleRate < 1 {
				if rand.Float64() >= c.sampleRate { //nolint:gosec // statistical sampling, not security
					continue
				}
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

		for stepName, latencies := range r.StepStats {
			for _, l := range latencies {
				if c.ctx != nil && c.ctx.Err() != nil {
					return
				}
				if c.sampleRate > 0 && c.sampleRate < 1 {
					if rand.Float64() >= c.sampleRate { //nolint:gosec // statistical sampling, not security
						continue
					}
				}
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
					baseTime.Add(time.Duration(pointIndex)*time.Microsecond),
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
