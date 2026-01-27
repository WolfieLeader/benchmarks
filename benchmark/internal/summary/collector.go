package summary

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"benchmark-client/internal/client"
	"benchmark-client/internal/config"
	"benchmark-client/internal/container"
)

type ServerResult struct {
	Name        string                   `json:"name"`
	ContainerID string                   `json:"-"`
	ImageName   string                   `json:"-"`
	Port        int                      `json:"-"`
	StartTime   time.Time                `json:"-"`
	EndTime     time.Time                `json:"-"`
	Duration    time.Duration            `json:"-"`
	Endpoints   []client.EndpointResult  `json:"-"`
	Error       string                   `json:"-"`
	Resources   *container.ResourceStats `json:"-"`
	Capacity    *client.CapacityResult   `json:"-"`
}

type MetaResults struct {
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
	Name       string                   `json:"name"`
	DurationMs int64                    `json:"duration_ms"`
	Error      string                   `json:"error,omitempty"`
	Stats      *StatsSummary            `json:"stats,omitempty"`
	Endpoints  []EndpointSummary        `json:"endpoints,omitempty"`
	Resources  *container.ResourceStats `json:"resources,omitempty"`
	Capacity   *client.CapacityResult   `json:"capacity,omitempty"`
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

type Writer struct {
	mu         sync.Mutex
	startTime  time.Time
	config     *config.GlobalConfig
	resultsDir string
}

func NewWriter(cfg *config.GlobalConfig, resultsDir string) *Writer {
	return &Writer{
		startTime:  time.Now(),
		config:     cfg,
		resultsDir: resultsDir,
	}
}

func (w *Writer) ExportServerResult(result *ServerResult) (string, error) {
	summary := serverSummaryFromResult(result, w.config.RequestsPerEndpoint)

	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal server results: %w", err)
	}

	if err = os.MkdirAll(w.resultsDir, 0o750); err != nil {
		return "", fmt.Errorf("failed to create results dir: %w", err)
	}

	path := filepath.Join(w.resultsDir, result.Name+".json")
	if err = os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("failed to write server results: %w", err)
	}

	return path, nil
}

func (w *Writer) ExportMetaResults() (*MetaResults, []ServerSummary, string, error) {
	if err := os.MkdirAll(w.resultsDir, 0o750); err != nil {
		return nil, nil, "", fmt.Errorf("failed to create results dir: %w", err)
	}

	servers, successCount, failCount, err := readServerSummaries(w.resultsDir)
	if err != nil {
		return nil, nil, "", err
	}

	// Create server summaries for the main results file (without endpoint details)
	serverSummaries := make([]ServerSummary, 0, len(servers))
	for _, s := range servers {
		serverSummaries = append(serverSummaries, ServerSummary{
			Name:       s.Name,
			DurationMs: s.DurationMs,
			Error:      s.Error,
			Stats:      s.Stats,
			Resources:  s.Resources,
			Capacity:   s.Capacity,
		})
	}

	meta := w.meta()
	summary := BenchmarkSummary{
		TotalServers:      len(servers),
		SuccessfulServers: successCount,
		FailedServers:     failCount,
		TotalDurationMs:   time.Since(w.startTime).Milliseconds(),
	}

	metaResults := &MetaResults{
		Meta:    meta,
		Summary: summary,
		Servers: serverSummaries,
	}

	data, err := json.MarshalIndent(metaResults, "", "  ")
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to marshal meta results: %w", err)
	}

	path := filepath.Join(w.resultsDir, "results.json")
	if err = os.WriteFile(path, data, 0o600); err != nil {
		return nil, nil, "", fmt.Errorf("failed to write meta results: %w", err)
	}

	return metaResults, servers, path, nil
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

func (w *Writer) meta() ResultMeta {
	w.mu.Lock()
	defer w.mu.Unlock()
	return ResultMeta{
		Timestamp: w.startTime,
		Config: ResultConfig{
			BaseURL:             w.config.BaseURL,
			Workers:             w.config.Workers,
			RequestsPerEndpoint: w.config.RequestsPerEndpoint,
		},
	}
}

func readServerSummaries(dir string) (servers []ServerSummary, successCount, failCount int, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to read results dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "results.json" || !strings.HasSuffix(name, ".json") {
			continue
		}
		path := filepath.Join(dir, name)
		var data []byte
		data, err = os.ReadFile(path) //nolint:gosec // path is constructed from controlled results directory
		if err != nil {
			return nil, 0, 0, fmt.Errorf("failed to read %s: %w", path, err)
		}
		var server ServerSummary
		if err = json.Unmarshal(data, &server); err != nil {
			return nil, 0, 0, fmt.Errorf("failed to parse %s: %w", path, err)
		}
		servers = append(servers, server)
		if server.Error == "" {
			successCount++
		} else {
			failCount++
		}
	}

	return servers, successCount, failCount, nil
}

func serverSummaryFromResult(result *ServerResult, iterations int) ServerSummary {
	endpoints := make([]EndpointSummary, 0, len(result.Endpoints))
	for _, ep := range result.Endpoints {
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

	return ServerSummary{
		Name:       result.Name,
		DurationMs: result.Duration.Milliseconds(),
		Error:      result.Error,
		Stats:      aggregateEndpointStats(result.Endpoints, iterations),
		Endpoints:  endpoints,
		Resources:  result.Resources,
		Capacity:   result.Capacity,
	}
}

func aggregateEndpointStats(endpoints []client.EndpointResult, iterations int) *StatsSummary {
	if len(endpoints) == 0 {
		return nil
	}

	var (
		totalAvg, totalP50, totalP95, totalP99 time.Duration
		minLatency                             = time.Hour
		maxLatency                             time.Duration
		successCount                           int
		endpointCount                          int
	)

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
