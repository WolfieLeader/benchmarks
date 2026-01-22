package results

import (
	"benchmark-client/internal/client"
	"benchmark-client/internal/config"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// ServerResult contains benchmark results for a single server
type ServerResult struct {
	Name         string                  `json:"name"`
	ContainerID  string                  `json:"container_id"`
	ImageName    string                  `json:"image_name"`
	Port         int                     `json:"port"`
	StartTime    time.Time               `json:"start_time"`
	EndTime      time.Time               `json:"end_time"`
	Duration     string                  `json:"duration"`
	Endpoints    []client.EndpointResult `json:"endpoints"`
	OverallStats *OverallStats           `json:"overall_stats"`
	Error        string                  `json:"error,omitempty"`
}

// OverallStats contains aggregated statistics across all endpoints
type OverallStats struct {
	TotalRequests int     `json:"total_requests"`
	SuccessCount  int     `json:"success_count"`
	FailureCount  int     `json:"failure_count"`
	SuccessRate   float64 `json:"success_rate"`
	AvgLatency    string  `json:"avg_latency"`
	MinLatency    string  `json:"min_latency"`
	MaxLatency    string  `json:"max_latency"`
	P50Latency    string  `json:"p50_latency"`
	P95Latency    string  `json:"p95_latency"`
	P99Latency    string  `json:"p99_latency"`
	AvgLatencyNs  int64   `json:"avg_latency_ns"`
	MinLatencyNs  int64   `json:"min_latency_ns"`
	MaxLatencyNs  int64   `json:"max_latency_ns"`
	P50LatencyNs  int64   `json:"p50_latency_ns"`
	P95LatencyNs  int64   `json:"p95_latency_ns"`
	P99LatencyNs  int64   `json:"p99_latency_ns"`
	EndpointCount int     `json:"endpoint_count"`
	TestCaseCount int     `json:"test_case_count"`
}

// BenchmarkResults contains all benchmark results
type BenchmarkResults struct {
	Timestamp time.Time            `json:"timestamp"`
	Config    *config.GlobalConfig `json:"config"`
	Servers   []ServerResult       `json:"servers"`
	Summary   *BenchmarkSummary    `json:"summary"`
}

// BenchmarkSummary contains overall benchmark summary
type BenchmarkSummary struct {
	TotalServers      int    `json:"total_servers"`
	SuccessfulServers int    `json:"successful_servers"`
	FailedServers     int    `json:"failed_servers"`
	TotalDuration     string `json:"total_duration"`
}

// Collector aggregates benchmark results
type Collector struct {
	mu        sync.Mutex
	startTime time.Time
	config    *config.GlobalConfig
	servers   []ServerResult
}

// NewCollector creates a new results collector
func NewCollector(config *config.GlobalConfig) *Collector {
	return &Collector{
		startTime: time.Now(),
		config:    config,
		servers:   make([]ServerResult, 0),
	}
}

// AddServerResult adds a server's benchmark results
func (c *Collector) AddServerResult(result *ServerResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.servers = append(c.servers, *result)
}

// GetResults returns all collected results
func (c *Collector) GetResults() *BenchmarkResults {
	c.mu.Lock()
	defer c.mu.Unlock()

	totalDuration := time.Since(c.startTime)

	var successCount, failCount int
	for _, s := range c.servers {
		if s.Error == "" {
			successCount++
		} else {
			failCount++
		}
	}

	return &BenchmarkResults{
		Timestamp: c.startTime,
		Config:    c.config,
		Servers:   c.servers,
		Summary: &BenchmarkSummary{
			TotalServers:      len(c.servers),
			SuccessfulServers: successCount,
			FailedServers:     failCount,
			TotalDuration:     totalDuration.String(),
		},
	}
}

// Export writes the results to a JSON file
func (c *Collector) Export(filename string) error {
	results := c.GetResults()

	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal results: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write results file: %w", err)
	}

	return nil
}

// AggregateEndpointStats calculates overall stats from endpoint results
func AggregateEndpointStats(endpoints []client.EndpointResult, iterations int) *OverallStats {
	if len(endpoints) == 0 {
		return &OverallStats{}
	}

	var totalLatency, minLatency, maxLatency time.Duration
	var totalP50, totalP95, totalP99 time.Duration
	minLatency = time.Hour // Start with a large value
	var successCount, testCaseCount int

	for _, ep := range endpoints {
		if ep.Stats != nil {
			totalLatency += ep.Stats.Avg
			totalP50 += ep.Stats.P50
			totalP95 += ep.Stats.P95
			totalP99 += ep.Stats.P99
			if ep.Stats.Low > 0 && ep.Stats.Low < minLatency {
				minLatency = ep.Stats.Low
			}
			if ep.Stats.High > maxLatency {
				maxLatency = ep.Stats.High
			}
			successCount += int(ep.Stats.SuccessRate * float64(iterations))
		}
		testCaseCount += len(ep.TestCases)
	}

	avgLatency := time.Duration(0)
	avgP50 := time.Duration(0)
	avgP95 := time.Duration(0)
	avgP99 := time.Duration(0)
	if len(endpoints) > 0 {
		avgLatency = totalLatency / time.Duration(len(endpoints))
		avgP50 = totalP50 / time.Duration(len(endpoints))
		avgP95 = totalP95 / time.Duration(len(endpoints))
		avgP99 = totalP99 / time.Duration(len(endpoints))
	}

	if minLatency == time.Hour {
		minLatency = 0
	}

	totalRequests := iterations * len(endpoints)
	failureCount := totalRequests - successCount

	return &OverallStats{
		TotalRequests: totalRequests,
		SuccessCount:  successCount,
		FailureCount:  failureCount,
		SuccessRate:   float64(successCount) / float64(totalRequests),
		AvgLatency:    avgLatency.String(),
		MinLatency:    minLatency.String(),
		MaxLatency:    maxLatency.String(),
		P50Latency:    avgP50.String(),
		P95Latency:    avgP95.String(),
		P99Latency:    avgP99.String(),
		AvgLatencyNs:  avgLatency.Nanoseconds(),
		MinLatencyNs:  minLatency.Nanoseconds(),
		MaxLatencyNs:  maxLatency.Nanoseconds(),
		P50LatencyNs:  avgP50.Nanoseconds(),
		P95LatencyNs:  avgP95.Nanoseconds(),
		P99LatencyNs:  avgP99.Nanoseconds(),
		EndpointCount: len(endpoints),
		TestCaseCount: testCaseCount,
	}
}

// Complete marks the server result as complete
func (r *ServerResult) Complete(endpoints []client.EndpointResult, iterations int) {
	r.EndTime = time.Now()
	r.Duration = r.EndTime.Sub(r.StartTime).String()
	r.Endpoints = endpoints
	r.OverallStats = AggregateEndpointStats(endpoints, iterations)
}

// SetError marks the server result as failed
func (r *ServerResult) SetError(err error) {
	r.EndTime = time.Now()
	r.Duration = r.EndTime.Sub(r.StartTime).String()
	r.Error = err.Error()
}
