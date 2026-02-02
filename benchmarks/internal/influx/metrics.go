package influx

import (
	"math/rand"
	"time"

	"github.com/InfluxCommunity/influxdb3-go/influxdb3"

	"benchmark-client/internal/client"
	"benchmark-client/internal/container"
)

const writeBatchSize = 5000

//nolint:contextcheck // uses stored context from Client
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
			if rand.Float64() >= c.sampleRate { //nolint:gosec // statistical sampling, not security
				continue
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
				c.writePoints(points)
				points = points[:0]
			}
		}
	}
	c.writePoints(points)
}

//nolint:contextcheck // uses stored context from Client
func (c *Client) WriteFlowLatencies(runId, server string, results []client.TimedFlowResult) {
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
			if rand.Float64() >= c.sampleRate { //nolint:gosec // statistical sampling, not security
				continue
			}
			points = append(points, influxdb3.NewPoint(
				"flow_latency",
				map[string]string{
					"run_id":   runId,
					"server":   server,
					"flow_id":  r.FlowId,
					"database": r.Database,
				},
				map[string]any{
					"server_offset_ms":  l.ServerOffset.Milliseconds(),
					"flow_offset_ms":    l.EndpointOffset.Milliseconds(),
					"total_duration_ns": l.Duration.Nanoseconds(),
				},
				baseTime.Add(time.Duration(pointIndex)*time.Microsecond),
			))
			pointIndex++
			if len(points) >= writeBatchSize {
				c.writePoints(points)
				points = points[:0]
			}
		}

		for stepName, latencies := range r.StepStats {
			for _, l := range latencies {
				if c.ctx != nil && c.ctx.Err() != nil {
					return
				}
				if rand.Float64() >= c.sampleRate { //nolint:gosec // statistical sampling, not security
					continue
				}
				points = append(points, influxdb3.NewPoint(
					"flow_step_latency",
					map[string]string{
						"run_id":   runId,
						"server":   server,
						"flow_id":  r.FlowId,
						"database": r.Database,
						"step":     stepName,
					},
					map[string]any{
						"server_offset_ms": l.ServerOffset.Milliseconds(),
						"latency_ns":       l.Duration.Nanoseconds(),
					},
					baseTime.Add(time.Duration(pointIndex)*time.Microsecond),
				))
				pointIndex++
				if len(points) >= writeBatchSize {
					c.writePoints(points)
					points = points[:0]
				}
			}
		}
	}
	c.writePoints(points)
}

//nolint:contextcheck // uses stored context from Client
func (c *Client) WriteCapacityResult(runId, server string, result *client.CapacityResult) {
	if c == nil || result == nil {
		return
	}

	c.WritePoint("capacity_result",
		map[string]string{
			"run_id": runId,
			"server": server,
		},
		map[string]any{
			"max_workers":  result.MaxWorkersPassed,
			"achieved_rps": result.AchievedRPS,
			"p99_ms":       result.P99Ms,
			"success_rate": result.SuccessRate,
		},
		time.Now(),
	)
}

//nolint:contextcheck // uses stored context from Client
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
			"cpu_min_percent":  stats.CPU.MinPercent,
			"cpu_avg_percent":  stats.CPU.AvgPercent,
			"cpu_max_percent":  stats.CPU.MaxPercent,
			"samples":          stats.Samples,
		},
		time.Now(),
	)
}
