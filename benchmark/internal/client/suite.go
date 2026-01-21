package client

import (
	"benchmark-client/internal/config"
	"context"
	"fmt"
	"io"
	"net/textproto"
	"slices"
	"strings"
	"sync"
	"time"
)

// Suite manages benchmark execution across multiple endpoints
type Suite struct {
	client    *Client
	config    *config.Config
	serverURL string
	timeout   time.Duration
}

// NewSuite creates a new benchmark suite
func NewSuite(ctx context.Context, cfg *config.Config, serverURL string) *Suite {
	return &Suite{
		client:    New(ctx, serverURL),
		config:    cfg,
		serverURL: serverURL,
		timeout:   cfg.Global.GetTimeout(),
	}
}

// EndpointResult contains results for all test cases of an endpoint
type EndpointResult struct {
	Name      string           `json:"name"`
	Path      string           `json:"path"`
	Method    string           `json:"method"`
	TestCases []TestCaseResult `json:"test_cases"`
	Stats     *Stats           `json:"stats"`
	Error     string           `json:"error,omitempty"`
}

// TestCaseResult contains results for a single test case
type TestCaseResult struct {
	Name    string        `json:"name"`
	Success bool          `json:"success"`
	Error   string        `json:"error,omitempty"`
	Latency time.Duration `json:"latency"`
}

// RunAll executes all endpoints with the specified concurrency settings
func (s *Suite) RunAll(workers, iterations int) ([]EndpointResult, error) {
	results := make([]EndpointResult, 0, len(s.config.Endpoints))

	for name, endpoint := range s.config.Endpoints {
		endpointCopy := endpoint // Create copy for closure
		result, err := s.RunEndpoint(name, &endpointCopy, workers, iterations)
		if err != nil {
			results = append(results, EndpointResult{
				Name:   name,
				Path:   endpoint.Path,
				Method: endpoint.Method,
				Error:  err.Error(),
			})
			continue
		}
		results = append(results, *result)
	}

	return results, nil
}

// RunEndpoint executes all test cases for a single endpoint
func (s *Suite) RunEndpoint(name string, endpoint *config.EndpointConfig, workers, iterations int) (*EndpointResult, error) {
	// Resolve test cases from endpoint config
	resolvedCases := config.ResolveEndpointTestCases(endpoint)

	// Convert to executable testcases
	executableCases, err := s.convertToExecutable(resolvedCases)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare test cases: %w", err)
	}

	if len(executableCases) == 0 {
		return nil, fmt.Errorf("no valid test cases for endpoint %s", name)
	}

	// Run benchmark
	stats, testResults, err := s.runTestCases(executableCases, workers, iterations)
	if err != nil {
		return nil, err
	}

	return &EndpointResult{
		Name:      name,
		Path:      endpoint.Path,
		Method:    endpoint.Method,
		TestCases: testResults,
		Stats:     stats,
	}, nil
}

func (s *Suite) convertToExecutable(resolved []*config.ResolvedTestCase) ([]*ExecutableTestcase, error) {
	executable := make([]*ExecutableTestcase, 0, len(resolved))

	for _, tc := range resolved {
		exec, err := s.buildExecutable(tc)
		if err != nil {
			return nil, fmt.Errorf("test case %q: %w", tc.Name, err)
		}
		executable = append(executable, exec)
	}

	return executable, nil
}

func (s *Suite) buildExecutable(tc *config.ResolvedTestCase) (*ExecutableTestcase, error) {
	// Build URL with query parameters
	fullURL, err := BuildURLWithQuery(s.serverURL, tc.Path, tc.Query)
	if err != nil {
		return nil, err
	}

	exec := &ExecutableTestcase{
		Name:            tc.Name,
		URL:             fullURL,
		Method:          strings.ToUpper(tc.Method),
		Headers:         canonicalizeHeaders(tc.Headers),
		ExpectedStatus:  tc.ExpectedStatus,
		ExpectedHeaders: canonicalizeHeaders(tc.ExpectedHeaders),
		ExpectedBody:    tc.ExpectedBody,
		ExpectedText:    tc.ExpectedText,
	}

	// Determine request type and prepare body
	if tc.File != nil {
		// File upload (multipart)
		exec.RequestType = RequestTypeMultipart
		exec.MultipartFields = tc.FormData
		exec.FileUpload = &FileUpload{
			FieldName:   tc.File.FieldName,
			Filename:    tc.File.Filename,
			Content:     []byte(tc.File.Content),
			ContentType: tc.File.ContentType,
		}
	} else if len(tc.FormData) > 0 {
		// Form data - check if we need multipart or urlencoded
		// Default to urlencoded for simple form data
		exec.RequestType = RequestTypeForm
		exec.FormData = tc.FormData
	} else if tc.Body != nil {
		// JSON body
		exec.RequestType = RequestTypeJSON
		body, err := PrepareJSONBody(tc.Body)
		if err != nil {
			return nil, err
		}
		exec.Body = body
	} else {
		exec.RequestType = RequestTypeNone
	}

	return exec, nil
}

func canonicalizeHeaders(headers map[string]string) map[string]string {
	if headers == nil {
		return nil
	}
	result := make(map[string]string, len(headers))
	for k, v := range headers {
		key := textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(k))
		if key != "" {
			result[key] = strings.TrimSpace(v)
		}
	}
	return result
}

func (s *Suite) runTestCases(testcases []*ExecutableTestcase, workers, iterations int) (*Stats, []TestCaseResult, error) {
	if workers > iterations {
		workers = iterations
	}

	// Channel for distributing work
	workCh := make(chan *ExecutableTestcase)
	go func() {
		defer close(workCh)
		for i := range iterations {
			index := i % len(testcases)
			workCh <- testcases[index]
		}
	}()

	// Results collection
	type result struct {
		tcName  string
		latency time.Duration
		err     error
	}
	resultsCh := make(chan result)

	// Fan-out workers
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for tc := range workCh {
				latency, err := s.executeTestcase(tc)
				resultsCh <- result{
					tcName:  tc.Name,
					latency: latency,
					err:     err,
				}
			}
		}()
	}

	// Close results channel when all workers done
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	// Collect results
	testCaseResults := make(map[string]*TestCaseResult)
	for _, tc := range testcases {
		testCaseResults[tc.Name] = &TestCaseResult{
			Name:    tc.Name,
			Success: true,
		}
	}

	var count int
	var totalLatency, high, low time.Duration
	low = time.Hour
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

		if tcr, ok := testCaseResults[r.tcName]; ok {
			if tcr.Latency == 0 || r.latency < tcr.Latency {
				tcr.Latency = r.latency
			}
		}
	}

	// Build stats
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

	stats := &Stats{
		Avg:         avg,
		High:        high,
		Low:         low,
		P50:         p50,
		P95:         p95,
		P99:         p99,
		SuccessRate: float64(count) / float64(iterations),
	}

	// Convert map to slice
	results := make([]TestCaseResult, 0, len(testCaseResults))
	for _, tcr := range testCaseResults {
		results = append(results, *tcr)
	}

	return stats, results, nil
}

func (s *Suite) executeTestcase(tc *ExecutableTestcase) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(s.client.ctx, s.timeout)
	defer cancel()

	req, err := s.client.BuildRequest(ctx, tc)
	if err != nil {
		return 0, err
	}

	start := time.Now()
	resp, err := s.client.httpClient.Do(req)
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

// percentile returns the value at the given percentile from a sorted slice
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
