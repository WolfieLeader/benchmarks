package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sync"
	"time"

	"benchmark-client/internal/config"
)

type Suite struct {
	ctx        context.Context
	httpClient *http.Client
	transport  *http.Transport
	server     *config.ResolvedServer
}

type Stats struct {
	Avg         time.Duration `json:"avg"`
	High        time.Duration `json:"high"`
	Low         time.Duration `json:"low"`
	P50         time.Duration `json:"p50"`
	P95         time.Duration `json:"p95"`
	P99         time.Duration `json:"p99"`
	SuccessRate float64       `json:"success_rate"`
}

func NewSuite(ctx context.Context, server *config.ResolvedServer) *Suite {
	transport := NewHTTPTransport(server.Workers)

	return &Suite{
		ctx:        ctx,
		httpClient: &http.Client{Transport: transport},
		transport:  transport,
		server:     server,
	}
}

type EndpointResult struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	Method       string `json:"method"`
	Stats        *Stats `json:"stats"`
	Error        string `json:"error,omitempty"`
	FailureCount int    `json:"failure_count,omitempty"`
	LastError    string `json:"last_error,omitempty"`
}

func (s *Suite) Close() {
	if s.transport != nil {
		s.transport.CloseIdleConnections()
	}
}

func (s *Suite) RunAll() ([]EndpointResult, error) {
	endpointTestcases := make(map[string][]*config.Testcase)
	for _, tc := range s.server.Testcases {
		endpointTestcases[tc.EndpointName] = append(endpointTestcases[tc.EndpointName], tc)
	}

	results := make([]EndpointResult, 0, len(endpointTestcases))
	used := make(map[string]struct{}, len(endpointTestcases))
	for _, endpointName := range s.server.EndpointOrder {
		testcases, ok := endpointTestcases[endpointName]
		if !ok || len(testcases) == 0 {
			continue
		}
		first := testcases[0]

		// Run warmup if enabled
		if s.server.WarmupEnabled {
			s.runWarmup(testcases)
		}

		results = append(results, s.runEndpoint(endpointName, first.Path, first.Method, testcases))
		used[endpointName] = struct{}{}
	}

	if len(used) != len(endpointTestcases) {
		leftovers := make([]string, 0, len(endpointTestcases)-len(used))
		for endpointName := range endpointTestcases {
			if _, ok := used[endpointName]; ok {
				continue
			}
			leftovers = append(leftovers, endpointName)
		}
		slices.Sort(leftovers)
		for _, endpointName := range leftovers {
			testcases := endpointTestcases[endpointName]
			if len(testcases) == 0 {
				continue
			}
			first := testcases[0]

			// Run warmup if enabled
			if s.server.WarmupEnabled {
				s.runWarmup(testcases)
			}

			results = append(results, s.runEndpoint(endpointName, first.Path, first.Method, testcases))
		}
	}

	return results, nil
}

func (s *Suite) runEndpoint(name, path, method string, testcases []*config.Testcase) EndpointResult {
	if len(testcases) == 0 {
		return EndpointResult{
			Name:   name,
			Path:   path,
			Method: method,
			Error:  "no test cases",
		}
	}

	stats, failureCount, lastErr := s.runTestcases(testcases)

	return EndpointResult{
		Name:         name,
		Path:         path,
		Method:       method,
		Stats:        stats,
		FailureCount: failureCount,
		LastError:    lastErr,
	}
}

func (s *Suite) runTestcases(testcases []*config.Testcase) (stats *Stats, failureCount int, lastError string) {
	workers := min(s.server.Workers, s.server.RequestsPerEndpoint)
	iterations := s.server.RequestsPerEndpoint

	workCh := make(chan *config.Testcase)
	go func() {
		defer close(workCh)
		for i := range iterations {
			select {
			case <-s.ctx.Done():
				return
			case workCh <- testcases[i%len(testcases)]:
			}
		}
	}()

	type result struct {
		latency time.Duration
		err     error
	}
	resultsCh := make(chan result, workers)

	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for tc := range workCh {
				latency, err := s.executeTestcase(tc)
				resultsCh <- result{latency: latency, err: err}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	var count int
	var totalLatency, high, low time.Duration
	low = time.Hour
	latencies := make([]time.Duration, 0, iterations)

	for r := range resultsCh {
		if r.err != nil {
			failureCount++
			lastError = r.err.Error()
			continue
		}

		count++
		totalLatency += r.latency
		high = max(high, r.latency)
		low = min(low, r.latency)
		latencies = append(latencies, r.latency)
	}

	var avg, p50, p95, p99 time.Duration
	if count > 0 {
		avg = totalLatency / time.Duration(count)
		slices.Sort(latencies)
		p50 = percentile(latencies, 50)
		p95 = percentile(latencies, 95)
		p99 = percentile(latencies, 99)
	}
	if low == time.Hour {
		low = 0
	}

	var successRate float64
	if iterations > 0 {
		successRate = float64(count) / float64(iterations)
	}

	return &Stats{
		Avg:         avg,
		High:        high,
		Low:         low,
		P50:         p50,
		P95:         p95,
		P99:         p99,
		SuccessRate: successRate,
	}, failureCount, lastError
}

func (s *Suite) executeTestcase(tc *config.Testcase) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(s.ctx, s.server.Timeout)
	defer cancel()

	req, err := BuildRequest(ctx, tc)
	if err != nil {
		return 0, err
	}

	start := time.Now()
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	closeErr := resp.Body.Close()
	if err != nil {
		return 0, fmt.Errorf("failed to read response: %w", err)
	}
	if closeErr != nil {
		return 0, closeErr
	}

	latency := time.Since(start)

	if err := ValidateResponse(tc, resp, body); err != nil {
		return latency, err
	}

	return latency, nil
}

func percentile(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}
	idx := (p * len(sorted)) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// FlowStats contains statistics for a flow execution
type FlowStats struct {
	FlowId      string          `json:"flow_id"`
	Database    string          `json:"database,omitempty"`
	TotalRuns   int             `json:"total_runs"`
	Successes   int             `json:"successes"`
	Failures    int             `json:"failures"`
	SuccessRate float64         `json:"success_rate"`
	AvgDuration time.Duration   `json:"avg_duration"`
	P50Duration time.Duration   `json:"p50_duration"`
	P95Duration time.Duration   `json:"p95_duration"`
	P99Duration time.Duration   `json:"p99_duration"`
	LastError   string          `json:"last_error,omitempty"`
	FailedStep  int             `json:"failed_step,omitempty"`
	StepCount   int             `json:"step_count"`
	Steps       []FlowStepStats `json:"steps,omitempty"`
}

// FlowStepStats contains per-step statistics within a flow
type FlowStepStats struct {
	Name   string        `json:"name"`
	Method string        `json:"method"`
	Path   string        `json:"path"`
	Avg    time.Duration `json:"avg"`
	P50    time.Duration `json:"p50"`
	P95    time.Duration `json:"p95"`
	P99    time.Duration `json:"p99"`
}

// RunFlows executes all configured flows
func (s *Suite) RunFlows(hostPort int) []FlowStats {
	if len(s.server.Flows) == 0 {
		return nil
	}

	baseURL := fmt.Sprintf("http://localhost:%d", hostPort)
	results := make([]FlowStats, 0, len(s.server.Flows))

	for _, flow := range s.server.Flows {
		stats := s.runFlow(baseURL, flow)
		results = append(results, stats)
	}

	return results
}

func (s *Suite) runFlow(baseURL string, flow *config.ResolvedFlow) FlowStats {
	workers := min(s.server.Workers, s.server.RequestsPerEndpoint)
	iterations := s.server.RequestsPerEndpoint
	stepCount := len(flow.Endpoints)

	type workItem struct {
		workerID int
		cycleNum int
	}

	workCh := make(chan workItem)
	go func() {
		defer close(workCh)
		for i := range iterations {
			select {
			case <-s.ctx.Done():
				return
			case workCh <- workItem{workerID: i % workers, cycleNum: i}:
			}
		}
	}()

	resultsCh := make(chan FlowResult, workers)

	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for item := range workCh {
				result := RunFlow(s.ctx, s.httpClient, baseURL, flow, item.workerID, item.cycleNum, s.server.Timeout)
				resultsCh <- result
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	var successes, failures int
	var totalDuration time.Duration
	var lastError string
	var failedStep int
	durations := make([]time.Duration, 0, iterations)

	// Per-step duration collection (only for successful flows)
	stepDurations := make([][]time.Duration, stepCount)
	for i := range stepDurations {
		stepDurations[i] = make([]time.Duration, 0, iterations)
	}

	for r := range resultsCh {
		if r.Success {
			successes++
			durations = append(durations, r.TotalDuration)
			// Collect per-step durations
			for i, d := range r.StepDurations {
				stepDurations[i] = append(stepDurations[i], d)
			}
		} else {
			failures++
			lastError = r.Error
			failedStep = r.FailedStep
		}
		totalDuration += r.TotalDuration
	}

	var avgDuration, p50, p95, p99 time.Duration
	if successes > 0 {
		avgDuration = totalDuration / time.Duration(successes+failures)
		slices.Sort(durations)
		p50 = percentile(durations, 50)
		p95 = percentile(durations, 95)
		p99 = percentile(durations, 99)
	}

	var successRate float64
	total := successes + failures
	if total > 0 {
		successRate = float64(successes) / float64(total)
	}

	// Calculate per-step statistics
	steps := make([]FlowStepStats, stepCount)
	for i, ep := range flow.Endpoints {
		steps[i] = FlowStepStats{
			Name:   ep.Name,
			Method: ep.Method,
			Path:   ep.Path,
		}
		if len(stepDurations[i]) > 0 {
			var stepTotal time.Duration
			for _, d := range stepDurations[i] {
				stepTotal += d
			}
			steps[i].Avg = stepTotal / time.Duration(len(stepDurations[i]))
			slices.Sort(stepDurations[i])
			steps[i].P50 = percentile(stepDurations[i], 50)
			steps[i].P95 = percentile(stepDurations[i], 95)
			steps[i].P99 = percentile(stepDurations[i], 99)
		}
	}

	return FlowStats{
		FlowId:      flow.Id,
		Database:    flow.Database,
		TotalRuns:   total,
		Successes:   successes,
		Failures:    failures,
		SuccessRate: successRate,
		AvgDuration: avgDuration,
		P50Duration: p50,
		P95Duration: p95,
		P99Duration: p99,
		LastError:   lastError,
		FailedStep:  failedStep,
		StepCount:   stepCount,
		Steps:       steps,
	}
}

// runWarmup executes warmup requests without recording latencies
func (s *Suite) runWarmup(testcases []*config.Testcase) {
	warmupRequests := s.server.WarmupRequestsPerTestcase * len(testcases)
	workers := min(s.server.Workers, warmupRequests)

	workCh := make(chan *config.Testcase)
	go func() {
		defer close(workCh)
		for i := range warmupRequests {
			select {
			case <-s.ctx.Done():
				return
			case workCh <- testcases[i%len(testcases)]:
			}
		}
	}()

	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for tc := range workCh {
				_, _ = s.executeTestcase(tc) // Discard result
			}
		}()
	}
	wg.Wait()
}
