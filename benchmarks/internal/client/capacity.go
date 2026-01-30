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
	"benchmark-client/internal/printer"
)

type CapacityTester struct {
	ctx     context.Context
	config  config.CapacityConfig
	rootTC  *config.Testcase
	timeout time.Duration
}

type CapacityResult struct {
	MaxWorkersPassed int     `json:"max_workers_passed"`
	AchievedRPS      float64 `json:"achieved_rps"`
	P99Ms            float64 `json:"p99_ms"`
	SuccessRate      float64 `json:"success_rate"`
	Iterations       int     `json:"iterations"`
}

type iterationStats struct {
	passed      bool
	successRate float64
	p99         time.Duration
	rps         float64
	total       int
}

func NewCapacityTester(ctx context.Context, cfg *config.CapacityConfig, rootTC *config.Testcase, timeout time.Duration) *CapacityTester {
	return &CapacityTester{
		ctx:     ctx,
		config:  *cfg,
		rootTC:  rootTC,
		timeout: timeout,
	}
}

func (ct *CapacityTester) Run() (*CapacityResult, error) {
	printer.Linef("Capacity: finding max workers (range %d-%d, precision %s)", ct.config.MinWorkers, ct.config.MaxWorkers, ct.config.Precision)
	printer.Blank()
	printer.CapacityTableHeader()

	low := ct.config.MinWorkers
	high := ct.config.MaxWorkers
	searchRange := high - low
	precisionWorkers := int(float64(searchRange) * ct.config.PrecisionPct / 100)
	precisionWorkers = max(precisionWorkers, 1)
	iterations := 0
	var bestStats iterationStats

	// First check: does min_workers pass? If not, result is 0.
	stats, err := ct.testWorkers(low, &iterations)
	if err != nil {
		return nil, err
	}
	if !stats.passed {
		return &CapacityResult{
			MaxWorkersPassed: 0,
			Iterations:       iterations,
		}, nil
	}
	bestStats = stats

	// Quick check: does max_workers pass? If so, skip the search.
	if low < high {
		stats, err = ct.testWorkers(high, &iterations)
		if err != nil {
			return nil, err
		}
		if stats.passed {
			low = high
			bestStats = stats
		} else {
			high--
		}
	}

	// Binary search: find the highest passing worker count
	for high-low > precisionWorkers {
		if ct.ctx.Err() != nil {
			return nil, ct.ctx.Err()
		}

		mid := (low + high + 1) / 2
		stats, err := ct.testWorkers(mid, &iterations)
		if err != nil {
			return nil, err
		}

		if stats.passed {
			low = mid
			bestStats = stats
		} else {
			high = mid - 1
		}
	}

	return &CapacityResult{
		MaxWorkersPassed: low,
		AchievedRPS:      bestStats.rps,
		P99Ms:            float64(bestStats.p99.Milliseconds()),
		SuccessRate:      bestStats.successRate,
		Iterations:       iterations,
	}, nil
}

func (ct *CapacityTester) testWorkers(workers int, iterations *int) (iterationStats, error) {
	if ct.ctx.Err() != nil {
		return iterationStats{}, ct.ctx.Err()
	}
	stats, err := ct.runIteration(workers)
	*iterations++
	if err != nil {
		return stats, err
	}
	printer.CapacityTableRow(workers, stats.passed, stats.rps, float64(stats.p99.Milliseconds()), stats.successRate)
	return stats, nil
}

func (ct *CapacityTester) runIteration(workers int) (iterationStats, error) {
	transport := NewHTTPTransport(workers)
	httpClient := &http.Client{Transport: transport}
	defer transport.CloseIdleConnections()

	// Warmup phase
	if ct.config.WarmupDuration > 0 {
		ct.runTimedPhase(httpClient, workers, ct.config.WarmupDuration)
	}

	// Measure phase
	return ct.measure(httpClient, workers, ct.config.MeasureDuration)
}

func (ct *CapacityTester) runTimedPhase(httpClient *http.Client, workers int, duration time.Duration) {
	ctx, cancel := context.WithTimeout(ct.ctx, duration)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				_, _ = ct.doRequest(ctx, httpClient)
			}
		}()
	}
	wg.Wait()
}

func (ct *CapacityTester) measure(httpClient *http.Client, workers int, duration time.Duration) (iterationStats, error) {
	ctx, cancel := context.WithTimeout(ct.ctx, duration)
	defer cancel()

	type result struct {
		latency time.Duration
		ok      bool
	}

	resultsCh := make(chan result, workers*100)

	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				latency, err := ct.doRequest(ctx, httpClient)
				if ctx.Err() != nil {
					return
				}
				resultsCh <- result{latency: latency, ok: err == nil}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	var successCount, totalCount int
	latencies := make([]time.Duration, 0, 10000)

	for r := range resultsCh {
		totalCount++
		if r.ok {
			successCount++
			latencies = append(latencies, r.latency)
		}
	}

	if totalCount == 0 {
		return iterationStats{}, nil
	}

	successRate := float64(successCount) / float64(totalCount)
	rps := float64(totalCount) / duration.Seconds()

	var p99 time.Duration
	if len(latencies) > 0 {
		slices.Sort(latencies)
		p99 = percentile(latencies, 99)
	}

	p99Ms := float64(p99) / float64(time.Millisecond)
	p99Threshold := float64(ct.config.P99ThresholdDur) / float64(time.Millisecond)
	passed := successRate >= ct.config.SuccessRatePct && p99Ms <= p99Threshold

	return iterationStats{
		passed:      passed,
		successRate: successRate,
		p99:         p99,
		rps:         rps,
		total:       totalCount,
	}, nil
}

func (ct *CapacityTester) doRequest(ctx context.Context, httpClient *http.Client) (time.Duration, error) {
	reqCtx, cancel := context.WithTimeout(ctx, ct.timeout)
	defer cancel()

	req, err := BuildRequest(reqCtx, ct.rootTC)
	if err != nil {
		return 0, err
	}

	start := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}

	_, err = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	closeErr := resp.Body.Close()
	latency := time.Since(start)
	if err != nil {
		return latency, err
	}
	if closeErr != nil {
		return latency, closeErr
	}

	if resp.StatusCode != ct.rootTC.ExpectedStatus {
		return latency, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	return latency, nil
}
