package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sync"
	"time"

	"benchmark-client/internal/config"
)

type ProgressCallbacks struct {
	OnEndpoint func(method, path string, done int)
	OnSequence func(seqName string, done int)
}

type Suite struct {
	ctx             context.Context
	httpClient      *http.Client
	transport       *http.Transport
	server          *config.ResolvedServer
	serverStartTime time.Time
	timedResults    []TimedResult
	timedSequences  []TimedSequenceResult
	progress        *ProgressCallbacks
}

func NewSuite(ctx context.Context, server *config.ResolvedServer, progress *ProgressCallbacks) *Suite {
	transport := NewHTTPTransport(server.Concurrency)

	return &Suite{
		ctx:        ctx,
		httpClient: &http.Client{Transport: transport},
		transport:  transport,
		server:     server,
		progress:   progress,
	}
}

type EndpointResult struct {
	Name          string `json:"name"`
	Path          string `json:"path"`
	Method        string `json:"method"`
	Database      string `json:"database,omitempty"`
	SequenceId    string `json:"sequence_id,omitempty"`
	Stats         *Stats `json:"stats"`
	Error         string `json:"error,omitempty"`
	FailureCount  int    `json:"failure_count,omitempty"`
	CanceledCount int    `json:"canceled_count,omitempty"`
	LastError     string `json:"last_error,omitempty"`
}

func (s *Suite) Close() {
	if s.transport != nil {
		s.transport.CloseIdleConnections()
		s.transport.DisableKeepAlives = true
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
	endpointsDone := 0

	for _, endpointName := range s.server.EndpointOrder {
		if s.ctx.Err() != nil {
			break
		}
		testcases, ok := endpointTestcases[endpointName]
		if !ok || len(testcases) == 0 {
			continue
		}
		endpointsDone = s.runEndpointWithWarmup(testcases, endpointsDone, &results)
		used[endpointName] = struct{}{}
	}

	if len(used) != len(endpointTestcases) && s.ctx.Err() == nil {
		leftovers := make([]string, 0, len(endpointTestcases)-len(used))
		for endpointName := range endpointTestcases {
			if _, ok := used[endpointName]; ok {
				continue
			}
			leftovers = append(leftovers, endpointName)
		}
		slices.Sort(leftovers)
		for _, endpointName := range leftovers {
			if s.ctx.Err() != nil {
				break
			}
			testcases := endpointTestcases[endpointName]
			if len(testcases) == 0 {
				continue
			}
			endpointsDone = s.runEndpointWithWarmup(testcases, endpointsDone, &results)
		}
	}

	return results, nil //nolint:nilerr // context cancellation returns partial results, not an error
}

func (s *Suite) runEndpointWithWarmup(testcases []*config.Testcase, done int, results *[]EndpointResult) int {
	if len(testcases) == 0 {
		return done
	}
	first := testcases[0]

	if s.progress != nil && s.progress.OnEndpoint != nil {
		s.progress.OnEndpoint(first.Method, first.Path, done)
	}

	if s.server.WarmupDuration > 0 {
		s.runWarmup(testcases)
		if s.ctx.Err() != nil {
			return done
		}
		if s.server.WarmupPause > 0 {
			time.Sleep(s.server.WarmupPause)
		}
	}

	*results = append(*results, s.runEndpoint(first.EndpointName, first.Path, first.Method, testcases))
	return done + 1
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

	stats, timedLatencies, failureCount, canceledCount, lastErr := s.runTestcases(testcases)

	s.timedResults = append(s.timedResults, TimedResult{
		Endpoint:  name,
		Method:    method,
		Latencies: timedLatencies,
	})

	return EndpointResult{
		Name:          name,
		Path:          path,
		Method:        method,
		Stats:         stats,
		FailureCount:  failureCount,
		CanceledCount: canceledCount,
		LastError:     lastErr,
	}
}

func (s *Suite) runTestcases(testcases []*config.Testcase) (stats *Stats, timedLatencies []TimedLatency, failureCount, canceledCount int, lastError string) {
	workers := s.server.Concurrency
	endpointStartTime := time.Now()

	ctx, cancel := context.WithTimeout(s.ctx, s.server.DurationPerEndpoint)
	defer cancel()

	workCh := make(chan *config.Testcase)
	go func() {
		defer close(workCh)
		index := 0
		for ctx.Err() == nil {
			select {
			case <-ctx.Done():
				return
			case workCh <- testcases[index%len(testcases)]:
				index++
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
				latency, err := s.executeTestcase(ctx, tc)
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
	latencies := make([]time.Duration, 0, 10000)
	timedLatencies = make([]TimedLatency, 0, 10000)

	for r := range resultsCh {
		if r.err != nil {
			if isBenchmarkContextCancellation(ctx, r.err) {
				canceledCount++
				continue
			}
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

	totalRequests := count + failureCount
	stats = CalculateStats(latencies, count, totalRequests)
	return stats, timedLatencies, failureCount, canceledCount, lastError
}

func (s *Suite) executeTestcase(ctx context.Context, tc *config.Testcase) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, s.server.RequestTimeout)
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

type SequenceStats struct {
	SequenceId  string              `json:"sequence_id"`
	Database    string              `json:"database,omitempty"`
	TotalRuns   int                 `json:"total_runs"`
	Successes   int                 `json:"successes"`
	Failures    int                 `json:"failures"`
	SuccessRate float64             `json:"success_rate"`
	AvgDuration time.Duration       `json:"avg_duration"`
	P50Duration time.Duration       `json:"p50_duration"`
	P95Duration time.Duration       `json:"p95_duration"`
	P99Duration time.Duration       `json:"p99_duration"`
	LastError   string              `json:"last_error,omitempty"`
	FailedStep  int                 `json:"failed_step,omitempty"`
	StepCount   int                 `json:"step_count"`
	Steps       []SequenceStepStats `json:"steps,omitempty"`
}

type SequenceStepStats struct {
	Name     string        `json:"name"`
	Method   string        `json:"method"`
	Path     string        `json:"path"`
	Count    int           `json:"count"`
	Attempts int           `json:"attempts"`
	Failures int           `json:"failures"`
	Avg      time.Duration `json:"avg"`
	Low      time.Duration `json:"low"`
	High     time.Duration `json:"high"`
	P50      time.Duration `json:"p50"`
	P95      time.Duration `json:"p95"`
	P99      time.Duration `json:"p99"`
}

func (s *Suite) RunSequences(hostPort int) []SequenceStats {
	if len(s.server.Sequences) == 0 {
		return nil
	}

	if s.transport != nil {
		s.transport.CloseIdleConnections()
	}

	s.timedSequences = nil
	baseUrl := fmt.Sprintf("http://localhost:%d", hostPort)
	results := make([]SequenceStats, 0, len(s.server.Sequences))

	for i, seq := range s.server.Sequences {
		if s.progress != nil && s.progress.OnSequence != nil {
			seqName := seq.Id
			if seq.Database != "" {
				seqName = fmt.Sprintf("%s/%s", seq.Id, seq.Database)
			}
			s.progress.OnSequence(seqName, i)
		}
		stats := s.runSequence(baseUrl, seq)
		results = append(results, stats)
	}

	return results
}

type timedSequenceResultItem struct {
	result         SequenceResult
	serverOffset   time.Duration
	sequenceOffset time.Duration
}

func (s *Suite) runSequence(baseURL string, seq *config.ResolvedSequence) SequenceStats {
	workers := s.server.Concurrency
	stepCount := len(seq.Endpoints)
	sequenceStartTime := time.Now()

	ctx, cancel := context.WithTimeout(s.ctx, s.server.DurationPerEndpoint*time.Duration(stepCount))
	defer cancel()

	type workItem struct {
		workerId int
		cycleNum int
	}

	workCh := make(chan workItem)
	go func() {
		defer close(workCh)
		index := 0
		for ctx.Err() == nil {
			select {
			case <-ctx.Done():
				return
			case workCh <- workItem{workerId: index % workers, cycleNum: index}:
				index++
			}
		}
	}()

	resultsCh := make(chan timedSequenceResultItem, workers)

	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for item := range workCh {
				requestStart := time.Now()
				serverOffset := requestStart.Sub(s.serverStartTime)
				sequenceOffset := requestStart.Sub(sequenceStartTime)
				result := RunSequence(ctx, s.httpClient, baseURL, seq, item.workerId, item.cycleNum, s.server.RequestTimeout)
				resultsCh <- timedSequenceResultItem{
					result:         result,
					serverOffset:   serverOffset,
					sequenceOffset: sequenceOffset,
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
	durations := make([]time.Duration, 0, 10000)

	stepDurations := make([][]time.Duration, stepCount)
	for i := range stepDurations {
		stepDurations[i] = make([]time.Duration, 0, 10000)
	}
	stepAttempts := make([]int, stepCount)
	stepFailures := make([]int, stepCount)

	timedLatencies := make([]TimedLatency, 0, 10000)
	stepTimedLatencies := make(map[string][]TimedLatency)
	for _, ep := range seq.Endpoints {
		stepTimedLatencies[ep.Name] = make([]TimedLatency, 0, 10000)
	}

	recordSteps := func(item timedSequenceResultItem, count int) {
		var stepOffset time.Duration
		for i := range count {
			d := item.result.StepDurations[i]
			stepAttempts[i]++
			stepDurations[i] = append(stepDurations[i], d)
			stepName := seq.Endpoints[i].Name
			stepTimedLatencies[stepName] = append(stepTimedLatencies[stepName], TimedLatency{
				ServerOffset:   item.serverOffset + stepOffset,
				EndpointOffset: item.sequenceOffset + stepOffset,
				Duration:       d,
			})
			stepOffset += d
		}
	}

	for item := range resultsCh {
		r := item.result
		if r.Success {
			successes++
			totalDuration += r.TotalDuration
			durations = append(durations, r.TotalDuration)
			recordSteps(item, len(r.StepDurations))
			timedLatencies = append(timedLatencies, TimedLatency{
				ServerOffset:   item.serverOffset,
				EndpointOffset: item.sequenceOffset,
				Duration:       r.TotalDuration,
			})
		} else {
			if r.ContextCanceled {
				continue
			}
			failures++
			lastError = r.Error
			failedStep = r.FailedStep
			recordSteps(item, min(r.FailedStep, stepCount))
			if r.FailedStep >= 0 && r.FailedStep < stepCount {
				stepAttempts[r.FailedStep]++
				stepFailures[r.FailedStep]++
			}
		}
	}

	s.timedSequences = append(s.timedSequences, TimedSequenceResult{
		SequenceId: seq.Id,
		Database:   seq.Database,
		Latencies:  timedLatencies,
		StepStats:  stepTimedLatencies,
	})

	var avgDuration, p50, p95, p99 time.Duration
	if successes > 0 {
		avgDuration = totalDuration / time.Duration(successes)
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

	steps := make([]SequenceStepStats, stepCount)
	for i, ep := range seq.Endpoints {
		steps[i] = SequenceStepStats{
			Name:   ep.Name,
			Method: ep.Method,
			Path:   ep.Path,
		}
		if len(stepDurations[i]) > 0 {
			var stepTotal time.Duration
			low := time.Hour
			var high time.Duration
			for _, d := range stepDurations[i] {
				stepTotal += d
				if d < low {
					low = d
				}
				if d > high {
					high = d
				}
			}
			steps[i].Count = len(stepDurations[i])
			steps[i].Avg = stepTotal / time.Duration(len(stepDurations[i]))
			steps[i].Low = low
			steps[i].High = high
			slices.Sort(stepDurations[i])
			steps[i].P50 = Percentile(stepDurations[i], 50)
			steps[i].P95 = Percentile(stepDurations[i], 95)
			steps[i].P99 = Percentile(stepDurations[i], 99)
		}
		steps[i].Attempts = stepAttempts[i]
		steps[i].Failures = stepFailures[i]
	}

	return SequenceStats{
		SequenceId:  seq.Id,
		Database:    seq.Database,
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
	if len(testcases) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(s.ctx, s.server.WarmupDuration)
	defer cancel()

	workers := min(s.server.Concurrency, len(testcases))
	if workers <= 0 {
		workers = 1
	}

	var wg sync.WaitGroup
	wg.Add(workers)
	for workerId := range workers {
		go func(id int) {
			defer wg.Done()
			index := id % len(testcases)
			for ctx.Err() == nil {
				_, _ = s.executeTestcase(ctx, testcases[index]) // Discard result
				index++
				if index >= len(testcases) {
					index = 0
				}
			}
		}(workerId)
	}

	wg.Wait()
}

func SequenceStepsToResults(sequences []SequenceStats) []EndpointResult {
	totalSteps := 0
	for i := range sequences {
		totalSteps += len(sequences[i].Steps)
	}
	results := make([]EndpointResult, 0, totalSteps)
	for i := range sequences {
		seq := &sequences[i]
		for j := range seq.Steps {
			step := &seq.Steps[j]
			var successRate float64
			if step.Attempts > 0 {
				successRate = float64(step.Count) / float64(step.Attempts)
			}
			var lastError string
			if step.Failures > 0 {
				lastError = seq.LastError
			}
			results = append(results, EndpointResult{
				Name:       step.Name,
				Path:       step.Path,
				Method:     step.Method,
				Database:   seq.Database,
				SequenceId: seq.SequenceId,
				Stats: &Stats{
					Count:       step.Count,
					TotalCount:  step.Attempts,
					Avg:         step.Avg,
					Low:         step.Low,
					High:        step.High,
					P50:         step.P50,
					P95:         step.P95,
					P99:         step.P99,
					SuccessRate: successRate,
				},
				FailureCount: step.Failures,
				LastError:    lastError,
			})
		}
	}
	return results
}

func (s *Suite) GetTimedResults() []TimedResult {
	return s.timedResults
}

func (s *Suite) GetTimedSequences() []TimedSequenceResult {
	return s.timedSequences
}

func isBenchmarkContextCancellation(benchmarkCtx context.Context, err error) bool {
	if benchmarkCtx == nil || benchmarkCtx.Err() == nil {
		return false
	}

	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
