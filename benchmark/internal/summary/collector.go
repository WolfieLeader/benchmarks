package summary

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"benchmark-client/internal/client"
	"benchmark-client/internal/config"
	"benchmark-client/internal/container"
)

// durationOpts: json/v2 has no default time.Duration representation (Marshal
// errors with a SemanticError), and the exported summaries embed raw
// client.SequenceStats/Stats with duration fields. These explicit v2
// marshalers keep the v1-compatible int64-nanosecond encoding on both write
// and read-back — pure json/v2 (repo canon), no v1-options shim.
// NB: pkg.go.dev's json/v2 page documents `format:nano` field tags for
// time.Duration, but go1.27rc1 rejects them ("unsupported `format` tag
// option"). Revisit when the Go pin moves (PLAN §10).
var durationOpts = json.JoinOptions(
	json.WithMarshalers(json.MarshalToFunc(func(enc *jsontext.Encoder, d time.Duration) error {
		return enc.WriteToken(jsontext.Int(int64(d)))
	})),
	json.WithUnmarshalers(json.UnmarshalFromFunc(func(dec *jsontext.Decoder, d *time.Duration) error {
		tok, err := dec.ReadToken()
		if err != nil {
			return err
		}
		if tok.Kind() != '0' {
			return fmt.Errorf("duration: expected JSON number, got %v", tok.Kind())
		}
		n, err := tok.Int()
		if err != nil {
			return fmt.Errorf("duration: %w", err)
		}
		*d = time.Duration(n)
		return nil
	})),
)

type ServerResult struct {
	Name        string                   `json:"name"`
	ContainerId string                   `json:"-"`
	ImageName   string                   `json:"-"`
	Port        int                      `json:"-"`
	StartTime   time.Time                `json:"-"`
	EndTime     time.Time                `json:"-"`
	Duration    time.Duration            `json:"-"`
	Results     []client.EndpointResult  `json:"-"`
	Sequences   []client.SequenceStats   `json:"-"`
	Error       string                   `json:"-"`
	Resources   *container.ResourceStats `json:"-"`
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
	BaseUrl             string `json:"base_url"`
	Concurrency         int    `json:"concurrency"`
	DurationPerEndpoint string `json:"duration_per_endpoint"`
	RequestTimeout      string `json:"request_timeout"`
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
	Results    []EndpointSummary        `json:"results,omitempty"`
	Sequences  []client.SequenceStats   `json:"sequences,omitempty"`
	Resources  *container.ResourceStats `json:"resources,omitempty"`
}

type EndpointSummary struct {
	Name          string        `json:"name"`
	Path          string        `json:"path"`
	Method        string        `json:"method"`
	Database      string        `json:"database,omitempty"`
	SequenceId    string        `json:"sequence_id,omitempty"`
	Error         string        `json:"error,omitempty"`
	Stats         *StatsSummary `json:"stats,omitempty"`
	Open          *OpenSummary  `json:"open,omitempty"` // open-model mode only
	FailureCount  int           `json:"failure_count,omitempty"`
	CanceledCount int           `json:"canceled_count,omitempty"`
	LastError     string        `json:"last_error,omitempty"`
}

type StatsSummary struct {
	Count       int     `json:"count"`
	TotalCount  int     `json:"total_count"`
	Rps         float64 `json:"rps"`
	AvgNs       int64   `json:"avg_ns"`
	P50Ns       int64   `json:"p50_ns,omitempty"`
	P95Ns       int64   `json:"p95_ns,omitempty"`
	P99Ns       int64   `json:"p99_ns,omitempty"`
	P999Ns      int64   `json:"p999_ns,omitempty"`
	MinNs       int64   `json:"min_ns"`
	MaxNs       int64   `json:"max_ns"`
	SuccessRate float64 `json:"success_rate"`
}

// OpenSummary is the export shape of client.OpenStats: open-model backpressure
// accounting plus the coordinated-omission-corrected response distribution.
// Schedule-lag durations are stored as *_ns int64 (like StatsSummary) rather
// than raw time.Duration so they need no custom marshaler.
type OpenSummary struct {
	TargetRate        float64       `json:"target_rate"`
	OfferedRate       float64       `json:"offered_rate"`
	Attempted         int           `json:"attempted"`
	DroppedIterations int           `json:"dropped_iterations"`
	MaxBacklog        int           `json:"max_backlog"`
	ScheduleLagP50Ns  int64         `json:"schedule_lag_p50_ns"`
	ScheduleLagP99Ns  int64         `json:"schedule_lag_p99_ns"`
	ScheduleLagMaxNs  int64         `json:"schedule_lag_max_ns"`
	Response          *StatsSummary `json:"response,omitempty"`
}

type Writer struct {
	startTime  time.Time
	config     *config.BenchmarkConfig
	resultsDir string
}

func NewWriter(cfg *config.BenchmarkConfig, resultsDir string) *Writer {
	return &Writer{
		startTime:  time.Now(),
		config:     cfg,
		resultsDir: resultsDir,
	}
}

func (w *Writer) ExportServerResult(result *ServerResult) (string, error) {
	summary := serverSummaryFromResult(result)

	data, err := json.Marshal(summary, jsontext.WithIndent("  "), durationOpts)
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

	serverSummaries := make([]ServerSummary, 0, len(servers))
	for _, s := range servers {
		serverSummaries = append(serverSummaries, ServerSummary{
			Name:       s.Name,
			DurationMs: s.DurationMs,
			Error:      s.Error,
			Stats:      s.Stats,
			Sequences:  s.Sequences,
			Resources:  s.Resources,
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

	data, err := json.Marshal(metaResults, jsontext.WithIndent("  "), durationOpts)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to marshal meta results: %w", err)
	}

	path := filepath.Join(w.resultsDir, "results.json")
	if err = os.WriteFile(path, data, 0o600); err != nil {
		return nil, nil, "", fmt.Errorf("failed to write meta results: %w", err)
	}

	return metaResults, servers, path, nil
}

func (r *ServerResult) Complete(results []client.EndpointResult) {
	r.EndTime = time.Now()
	r.Duration = r.EndTime.Sub(r.StartTime)
	r.Results = results
}

func (r *ServerResult) SetError(err error) {
	r.EndTime = time.Now()
	r.Duration = r.EndTime.Sub(r.StartTime)
	r.Error = err.Error()
}

func (w *Writer) meta() ResultMeta {
	return ResultMeta{
		Timestamp: w.startTime,
		Config: ResultConfig{
			BaseUrl:             w.config.BaseUrl,
			Concurrency:         w.config.Concurrency,
			DurationPerEndpoint: w.config.DurationPerEndpoint.String(),
			RequestTimeout:      w.config.RequestTimeout.String(),
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
		if err = json.Unmarshal(data, &server, durationOpts); err != nil {
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

func serverSummaryFromResult(result *ServerResult) ServerSummary {
	results := make([]EndpointSummary, 0, len(result.Results))
	for i := range result.Results {
		ep := &result.Results[i]
		results = append(results, EndpointSummary{
			Name:          ep.Name,
			Path:          ep.Path,
			Method:        ep.Method,
			Database:      ep.Database,
			SequenceId:    ep.SequenceId,
			Error:         ep.Error,
			Stats:         statsFromClient(ep.Stats),
			Open:          openFromClient(ep.Open),
			FailureCount:  ep.FailureCount,
			CanceledCount: ep.CanceledCount,
			LastError:     ep.LastError,
		})
	}

	return ServerSummary{
		Name:       result.Name,
		DurationMs: result.Duration.Milliseconds(),
		Error:      result.Error,
		Stats:      aggregateStats(result.Results),
		Results:    results,
		Sequences:  result.Sequences,
		Resources:  result.Resources,
	}
}

func aggregateStats(results []client.EndpointResult) *StatsSummary {
	if len(results) == 0 {
		return nil
	}

	var (
		totalLatency   time.Duration
		minLatency     = time.Hour
		maxLatency     time.Duration
		totalSuccesses int
		totalRequests  int
		resultCount    int
	)

	for i := range results {
		ep := &results[i]
		if ep.Stats == nil {
			continue
		}
		resultCount++
		totalLatency += time.Duration(ep.Stats.Count) * ep.Stats.Avg
		if ep.Stats.Low > 0 && ep.Stats.Low < minLatency {
			minLatency = ep.Stats.Low
		}
		if ep.Stats.High > maxLatency {
			maxLatency = ep.Stats.High
		}
		totalSuccesses += ep.Stats.Count
		totalRequests += ep.Stats.Count + ep.FailureCount
	}

	if resultCount == 0 {
		return nil
	}

	var avg time.Duration
	if totalSuccesses > 0 {
		avg = totalLatency / time.Duration(totalSuccesses)
	}
	if minLatency == time.Hour {
		minLatency = 0
	}

	var successRate float64
	if totalRequests > 0 {
		successRate = float64(totalSuccesses) / float64(totalRequests)
	}

	// Deliberately no server-level Rps rollup (left zero). Endpoints run
	// sequentially, so summing per-endpoint RPS overcounts (they never overlap)
	// and totalRequests / wall-duration undercounts (wall time includes warmup,
	// gaps, and container settle). Neither is a correct aggregate throughput, so
	// per-server RPS ranking is deferred to the metrics DB (PLAN §9.2 / #7),
	// which has the real per-endpoint windows. Per-endpoint Rps is exact and is
	// reported on each EndpointSummary.
	return &StatsSummary{
		Count:       totalSuccesses,
		TotalCount:  totalRequests,
		AvgNs:       avg.Nanoseconds(),
		MinNs:       minLatency.Nanoseconds(),
		MaxNs:       maxLatency.Nanoseconds(),
		SuccessRate: successRate,
	}
}

func statsFromClient(stats *client.Stats) *StatsSummary {
	if stats == nil {
		return nil
	}

	return &StatsSummary{
		Count:       stats.Count,
		TotalCount:  stats.TotalCount,
		Rps:         stats.Rps,
		AvgNs:       stats.Avg.Nanoseconds(),
		P50Ns:       stats.P50.Nanoseconds(),
		P95Ns:       stats.P95.Nanoseconds(),
		P99Ns:       stats.P99.Nanoseconds(),
		P999Ns:      stats.P999.Nanoseconds(),
		MinNs:       stats.Low.Nanoseconds(),
		MaxNs:       stats.High.Nanoseconds(),
		SuccessRate: stats.SuccessRate,
	}
}

func openFromClient(open *client.OpenStats) *OpenSummary {
	if open == nil {
		return nil
	}

	return &OpenSummary{
		TargetRate:        open.TargetRate,
		OfferedRate:       open.OfferedRate,
		Attempted:         open.Attempted,
		DroppedIterations: open.DroppedIterations,
		MaxBacklog:        open.MaxBacklog,
		ScheduleLagP50Ns:  open.ScheduleLagP50.Nanoseconds(),
		ScheduleLagP99Ns:  open.ScheduleLagP99.Nanoseconds(),
		ScheduleLagMaxNs:  open.ScheduleLagMax.Nanoseconds(),
		Response:          statsFromClient(open.Response),
	}
}
