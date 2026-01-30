package influx

import (
	"time"

	"benchmark-client/internal/client"
	"benchmark-client/internal/container"
)

// WriteEndpointLatencies writes raw endpoint latencies to InfluxDB.
func (c *Client) WriteEndpointLatencies(runId, server string, results []client.TimedResult) {
	if c == nil {
		return
	}

	baseTime := time.Now()
	for _, r := range results {
		for i, l := range r.Latencies {
			c.WritePoint("request_latency",
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
				baseTime.Add(time.Duration(i)*time.Microsecond),
			)
		}
	}
}

// WriteFlowLatencies writes raw flow latencies to InfluxDB.
func (c *Client) WriteFlowLatencies(runId, server string, results []client.TimedFlowResult) {
	if c == nil {
		return
	}

	baseTime := time.Now()
	for _, r := range results {
		// Write total flow durations
		for i, l := range r.Latencies {
			c.WritePoint("flow_latency",
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
				baseTime.Add(time.Duration(i)*time.Microsecond),
			)
		}

		// Write per-step latencies
		for stepName, latencies := range r.StepStats {
			for i, l := range latencies {
				c.WritePoint("flow_step_latency",
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
					baseTime.Add(time.Duration(i)*time.Microsecond),
				)
			}
		}
	}
}

// WriteCapacityResult writes capacity test results to InfluxDB.
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

// WriteResourceStats writes resource usage to InfluxDB.
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
