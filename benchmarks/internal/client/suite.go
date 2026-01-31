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
	ctx             context.Context
	httpClient      *http.Client
	transport       *http.Transport
	server          *config.ResolvedServer
	serverStartTime time.Time
	timedResults    []TimedResult
	timedFlows      []TimedFlowResult
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
	s.serverStartTime = time.Now()
	s.timedResults = nil

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

	stats, timedLatencies, failureCount, lastErr := s.runTestcases(testcases)

	s.timedResults = append(s.timedResults, TimedResult{
		Endpoint:  name,
		Method:    method,
		Latencies: timedLatencies,
	})

	return EndpointResult{
		Name:         name,
		Path:         path,
		Method:       method,
		Stats:        stats,
		FailureCount: failureCount,
		LastError:    lastErr,
	}
}

func (s *Suite) runTestcases(testcases []*config.Testcase) (stats *Stats, timedLatencies []TimedLatency, failureCount int, lastError string) {
	workers := min(s.server.Workers, s.server.RequestsPerEndpoint)
	iterations := s.server.RequestsPerEndpoint
	endpointStartTime := time.Now()

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
		latency        time.Duration
		serverOffset   time.Duration
		endpointOffset time.Duration
		err            error
	}
	resultsCh := make(chan result, workers)

	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for tc := range workCh {
				requestStart := time.Now()
				serverOffset := requestStart.Sub(s.serverStartTime)
				endpointOffset := requestStart.Sub(endpointStartTime)
				latency, err := s.executeTestcase(tc)
				resultsCh <- result{
					latency:        latency,
					serverOffset:   serverOffset,
					endpointOffset: endpointOffset,
					err:            err,
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	var count int
	latencies := make([]time.Duration, 0, iterations)
	timedLatencies = make([]TimedLatency, 0, iterations)

	for r := range resultsCh {
		if r.err != nil {
			failureCount++
			lastError = r.err.Error()
			continue
		}

		count++
		latencies = append(latencies, r.latency)
		timedLatencies = append(timedLatencies, TimedLatency{
			ServerOffset:   r.serverOffset,
			EndpointOffset: r.endpointOffset,
			Duration:       r.latency,
		})
	}

	stats = CalculateStats(latencies, count, iterations)
	return stats, timedLatencies, failureCount, lastError
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

type FlowStepStats struct {
	Name   string        `json:"name"`
	Method string        `json:"method"`
	Path   string        `json:"path"`
	Avg    time.Duration `json:"avg"`
	P50    time.Duration `json:"p50"`
	P95    time.Duration `json:"p95"`
	P99    time.Duration `json:"p99"`
}

func (s *Suite) RunFlows(hostPort int) []FlowStats {
	if len(s.server.Flows) == 0 {
		return nil
	}

	s.timedFlows = nil
	baseURL := fmt.Sprintf("http://localhost:%d", hostPort)
	results := make([]FlowStats, 0, len(s.server.Flows))

	for _, flow := range s.server.Flows {
		stats := s.runFlow(baseURL, flow)
		results = append(results, stats)
	}

	return results
}

type timedFlowResultItem struct {
	result       FlowResult
	serverOffset time.Duration
	flowOffset   time.Duration
}

func (s *Suite) runFlow(baseURL string, flow *config.ResolvedFlow) FlowStats {
	workers := min(s.server.Workers, s.server.RequestsPerEndpoint)
	iterations := s.server.RequestsPerEndpoint
	stepCount := len(flow.Endpoints)
	flowStartTime := time.Now()

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

	resultsCh := make(chan timedFlowResultItem, workers)

	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for item := range workCh {
				requestStart := time.Now()
				serverOffset := requestStart.Sub(s.serverStartTime)
				flowOffset := requestStart.Sub(flowStartTime)
				result := RunFlow(s.ctx, s.httpClient, baseURL, flow, item.workerID, item.cycleNum, s.server.Timeout)
				resultsCh <- timedFlowResultItem{
					result:       result,
					serverOffset: serverOffset,
					flowOffset:   flowOffset,
				}
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

	stepDurations := make([][]time.Duration, stepCount)
	for i := range stepDurations {
		stepDurations[i] = make([]time.Duration, 0, iterations)
	}

	timedLatencies := make([]TimedLatency, 0, iterations)
	stepTimedLatencies := make(map[string][]TimedLatency)
	for _, ep := range flow.Endpoints {
		stepTimedLatencies[ep.Name] = make([]TimedLatency, 0, iterations)
	}

	for item := range resultsCh {
		r := item.result
		if r.Success {
			successes++
			durations = append(durations, r.TotalDuration)
			for i, d := range r.StepDurations {
				stepDurations[i] = append(stepDurations[i], d)
			}
			timedLatencies = append(timedLatencies, TimedLatency{
				ServerOffset:   item.serverOffset,
				EndpointOffset: item.flowOffset,
				Duration:       r.TotalDuration,
			})
			var stepOffset time.Duration
			for i, d := range r.StepDurations {
				stepName := flow.Endpoints[i].Name
				stepTimedLatencies[stepName] = append(stepTimedLatencies[stepName], TimedLatency{
					ServerOffset:   item.serverOffset + stepOffset,
					EndpointOffset: item.flowOffset + stepOffset,
					Duration:       d,
				})
				stepOffset += d
			}
		} else {
			failures++
			lastError = r.Error
			failedStep = r.FailedStep
		}
		totalDuration += r.TotalDuration
	}

	s.timedFlows = append(s.timedFlows, TimedFlowResult{
		FlowId:    flow.Id,
		Database:  flow.Database,
		Latencies: timedLatencies,
		StepStats: stepTimedLatencies,
	})

	var avgDuration, p50, p95, p99 time.Duration
	if successes > 0 {
		avgDuration = totalDuration / time.Duration(successes+failures)
		slices.Sort(durations)
		p50 = Percentile(durations, 50)
		p95 = Percentile(durations, 95)
		p99 = Percentile(durations, 99)
	}

	var successRate float64
	total := successes + failures
	if total > 0 {
		successRate = float64(successes) / float64(total)
	}

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
			steps[i].P50 = Percentile(stepDurations[i], 50)
			steps[i].P95 = Percentile(stepDurations[i], 95)
			steps[i].P99 = Percentile(stepDurations[i], 99)
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

func (s *Suite) GetTimedResults() []TimedResult {
	return s.timedResults
}

func (s *Suite) GetTimedFlows() []TimedFlowResult {
	return s.timedFlows
}
