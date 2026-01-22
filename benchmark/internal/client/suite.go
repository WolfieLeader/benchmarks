package client

import (
	"benchmark-client/internal/config"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"slices"
	"sync"
	"time"
)

type Suite struct {
	ctx        context.Context
	httpClient *http.Client
	server     *config.ResolvedServer
}

type Stats struct {
	Avg         time.Duration   `json:"avg"`
	High        time.Duration   `json:"high"`
	Low         time.Duration   `json:"low"`
	P50         time.Duration   `json:"p50"`
	P95         time.Duration   `json:"p95"`
	P99         time.Duration   `json:"p99"`
	SuccessRate float64         `json:"success_rate"`
	Latencies   []time.Duration `json:"-"` // Raw latencies for aggregation (not serialized)
}

func NewSuite(ctx context.Context, server *config.ResolvedServer) *Suite {
	transport := &http.Transport{
		MaxIdleConns:        server.Workers,
		MaxIdleConnsPerHost: server.Workers,
		IdleConnTimeout:     90 * time.Second,
	}

	return &Suite{
		ctx:        ctx,
		httpClient: &http.Client{Transport: transport},
		server:     server,
	}
}

type EndpointResult struct {
	Name      string           `json:"name"`
	Path      string           `json:"path"`
	Method    string           `json:"method"`
	TestCases []TestCaseResult `json:"test_cases"`
	Stats     *Stats           `json:"stats"`
	Error     string           `json:"error,omitempty"`
}

type TestCaseResult struct {
	Name    string        `json:"name"`
	Success bool          `json:"success"`
	Error   string        `json:"error,omitempty"`
	Latency time.Duration `json:"latency"`
}

func (s *Suite) RunAll() ([]EndpointResult, error) {
	// Group testcases by endpoint
	endpointTestcases := make(map[string][]*config.Testcase)
	for _, tc := range s.server.Testcases {
		endpointTestcases[tc.EndpointName] = append(endpointTestcases[tc.EndpointName], tc)
	}

	results := make([]EndpointResult, 0, len(endpointTestcases))

	for endpointName, testcases := range endpointTestcases {
		if len(testcases) == 0 {
			continue
		}

		// Use first testcase for endpoint metadata
		first := testcases[0]
		result := s.runEndpoint(endpointName, first.URL, first.Method, testcases)
		results = append(results, result)
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

	stats, testResults := s.runTestcases(testcases)

	return EndpointResult{
		Name:      name,
		Path:      path,
		Method:    method,
		TestCases: testResults,
		Stats:     stats,
	}
}

func (s *Suite) runTestcases(testcases []*config.Testcase) (*Stats, []TestCaseResult) {
	workers := min(s.server.Workers, s.server.RequestsPerEndpoint)
	iterations := s.server.RequestsPerEndpoint

	// Work channel - generator sends testcases
	workCh := make(chan *config.Testcase)
	go func() {
		defer close(workCh)
		for i := range iterations {
			// Check context cancellation before sending
			if s.ctx.Err() != nil {
				return
			}
			select {
			case workCh <- testcases[i%len(testcases)]:
			case <-s.ctx.Done():
				return
			}
		}
	}()

	// Result collection
	type result struct {
		tcName  string
		latency time.Duration
		err     error
	}
	resultsCh := make(chan result, workers)

	// Worker pool
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for tc := range workCh {
				latency, err := s.executeTestcase(tc)
				resultsCh <- result{tcName: tc.Name, latency: latency, err: err}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	// Aggregate results
	testCaseResults := make(map[string]*TestCaseResult, len(testcases))
	for _, tc := range testcases {
		testCaseResults[tc.Name] = &TestCaseResult{Name: tc.Name, Success: true}
	}

	var count int
	var totalLatency, high time.Duration
	low := time.Duration(math.MaxInt64)
	latencies := make([]time.Duration, 0, iterations)

	for r := range resultsCh {
		if r.err != nil {
			if tcr, ok := testCaseResults[r.tcName]; ok {
				tcr.Success = false
				tcr.Error = r.err.Error()
			}
			continue
		}

		count++
		totalLatency += r.latency
		high = max(high, r.latency)
		low = min(low, r.latency)
		latencies = append(latencies, r.latency)

		if tcr, ok := testCaseResults[r.tcName]; ok && (tcr.Latency == 0 || r.latency < tcr.Latency) {
			tcr.Latency = r.latency
		}
	}

	// Calculate statistics
	var avg, p50, p95, p99 time.Duration
	if count > 0 {
		avg = totalLatency / time.Duration(count)
		slices.Sort(latencies)
		p50 = percentile(latencies, 50)
		p95 = percentile(latencies, 95)
		p99 = percentile(latencies, 99)
	}
	if low == time.Duration(math.MaxInt64) {
		low = 0
	}

	results := make([]TestCaseResult, 0, len(testCaseResults))
	for _, tcr := range testCaseResults {
		results = append(results, *tcr)
	}

	return &Stats{
		Avg:         avg,
		High:        high,
		Low:         low,
		P50:         p50,
		P95:         p95,
		P99:         p99,
		SuccessRate: float64(count) / float64(iterations),
		Latencies:   latencies,
	}, results
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
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return 0, fmt.Errorf("failed to read response: %w", err)
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
