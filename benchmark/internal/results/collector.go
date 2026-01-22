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

type ServerResult struct {
	Name        string                  `json:"name"`
	ContainerID string                  `json:"-"`
	ImageName   string                  `json:"-"`
	Port        int                     `json:"-"`
	StartTime   time.Time               `json:"-"`
	EndTime     time.Time               `json:"-"`
	Duration    time.Duration           `json:"-"`
	Endpoints   []client.EndpointResult `json:"-"`
	Error       string                  `json:"-"`
}

type BenchmarkResults struct {
	Meta    ResultMeta       `json:"meta"`
	Summary BenchmarkSummary `json:"summary"`
	Servers []ServerSummary  `json:"servers"`
}

type ResultMeta struct {
	Timestamp time.Time    `json:"timestamp"`
	Config    ResultConfig `json:"config"`
}

type ResultConfig struct {
	BaseURL             string `json:"base_url"`
	Workers             int    `json:"workers"`
	RequestsPerEndpoint int    `json:"requests_per_endpoint"`
}

type BenchmarkSummary struct {
	TotalServers      int   `json:"total_servers"`
	SuccessfulServers int   `json:"successful_servers"`
	FailedServers     int   `json:"failed_servers"`
	TotalDurationMs   int64 `json:"total_duration_ms"`
}

type ServerSummary struct {
	Name       string            `json:"name"`
	DurationMs int64             `json:"duration_ms"`
	Error      string            `json:"error,omitempty"`
	Stats      *StatsSummary     `json:"stats,omitempty"`
	Endpoints  []EndpointSummary `json:"endpoints,omitempty"`
}

type EndpointSummary struct {
	Name         string        `json:"name"`
	Path         string        `json:"path"`
	Method       string        `json:"method"`
	Error        string        `json:"error,omitempty"`
	Stats        *StatsSummary `json:"stats,omitempty"`
	FailureCount int           `json:"failure_count,omitempty"`
	LastError    string        `json:"last_error,omitempty"`
}

type StatsSummary struct {
	AvgNs       int64   `json:"avg_ns"`
	P50Ns       int64   `json:"p50_ns"`
	P95Ns       int64   `json:"p95_ns"`
	P99Ns       int64   `json:"p99_ns"`
	MinNs       int64   `json:"min_ns"`
	MaxNs       int64   `json:"max_ns"`
	SuccessRate float64 `json:"success_rate"`
}

type Collector struct {
	mu        sync.Mutex
	startTime time.Time
	config    *config.GlobalConfig
	servers   []ServerResult
}

func NewCollector(config *config.GlobalConfig) *Collector {
	return &Collector{
		startTime: time.Now(),
		config:    config,
		servers:   make([]ServerResult, 0),
	}
}

func (c *Collector) AddServerResult(result *ServerResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.servers = append(c.servers, *result)
}

func (c *Collector) GetResults() *BenchmarkResults {
	c.mu.Lock()
	defer c.mu.Unlock()

	var successCount, failCount int
	servers := make([]ServerSummary, 0, len(c.servers))

	for _, s := range c.servers {
		if s.Error == "" {
			successCount++
		} else {
			failCount++
		}

		endpoints := make([]EndpointSummary, 0, len(s.Endpoints))
		for _, ep := range s.Endpoints {
			endpoints = append(endpoints, EndpointSummary{
				Name:         ep.Name,
				Path:         ep.Path,
				Method:       ep.Method,
				Error:        ep.Error,
				Stats:        statsFromClient(ep.Stats),
				FailureCount: ep.FailureCount,
				LastError:    ep.LastError,
			})
		}

		servers = append(servers, ServerSummary{
			Name:       s.Name,
			DurationMs: s.Duration.Milliseconds(),
			Error:      s.Error,
			Stats:      aggregateEndpointStats(s.Endpoints, c.config.RequestsPerEndpoint),
			Endpoints:  endpoints,
		})
	}

	totalDuration := time.Since(c.startTime)

	return &BenchmarkResults{
		Meta: ResultMeta{
			Timestamp: c.startTime,
			Config: ResultConfig{
				BaseURL:             c.config.BaseURL,
				Workers:             c.config.Workers,
				RequestsPerEndpoint: c.config.RequestsPerEndpoint,
			},
		},
		Summary: BenchmarkSummary{
			TotalServers:      len(c.servers),
			SuccessfulServers: successCount,
			FailedServers:     failCount,
			TotalDurationMs:   totalDuration.Milliseconds(),
		},
		Servers: servers,
	}
}

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

func (r *ServerResult) Complete(endpoints []client.EndpointResult) {
	r.EndTime = time.Now()
	r.Duration = r.EndTime.Sub(r.StartTime)
	r.Endpoints = endpoints
}

func (r *ServerResult) SetError(err error) {
	r.EndTime = time.Now()
	r.Duration = r.EndTime.Sub(r.StartTime)
	r.Error = err.Error()
}

func aggregateEndpointStats(endpoints []client.EndpointResult, iterations int) *StatsSummary {
	if len(endpoints) == 0 {
		return nil
	}

	var totalAvg, totalP50, totalP95, totalP99 time.Duration
	var minLatency time.Duration = time.Hour
	var maxLatency time.Duration
	var successCount int
	var endpointCount int

	for _, ep := range endpoints {
		if ep.Stats == nil {
			continue
		}
		endpointCount++
		totalAvg += ep.Stats.Avg
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

	if endpointCount == 0 {
		return nil
	}

	avg := totalAvg / time.Duration(endpointCount)
	p50 := totalP50 / time.Duration(endpointCount)
	p95 := totalP95 / time.Duration(endpointCount)
	p99 := totalP99 / time.Duration(endpointCount)
	if minLatency == time.Hour {
		minLatency = 0
	}

	totalRequests := iterations * endpointCount

	return &StatsSummary{
		AvgNs:       avg.Nanoseconds(),
		P50Ns:       p50.Nanoseconds(),
		P95Ns:       p95.Nanoseconds(),
		P99Ns:       p99.Nanoseconds(),
		MinNs:       minLatency.Nanoseconds(),
		MaxNs:       maxLatency.Nanoseconds(),
		SuccessRate: float64(successCount) / float64(totalRequests),
	}
}

func statsFromClient(stats *client.Stats) *StatsSummary {
	if stats == nil {
		return nil
	}

	return &StatsSummary{
		AvgNs:       stats.Avg.Nanoseconds(),
		P50Ns:       stats.P50.Nanoseconds(),
		P95Ns:       stats.P95.Nanoseconds(),
		P99Ns:       stats.P99.Nanoseconds(),
		MinNs:       stats.Low.Nanoseconds(),
		MaxNs:       stats.High.Nanoseconds(),
		SuccessRate: stats.SuccessRate,
	}
}
