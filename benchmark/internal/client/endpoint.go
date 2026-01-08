package client

import (
	"fmt"
	"sync"
	"time"
)

var methods = []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}

type Endpoint struct {
	Path      string
	Method    string
	Headers   map[string]string
	Body      map[string]any
	Testcases []*Testcase
}

type Testcase struct {
	Path            string
	OverrideHeaders map[string]string
	OverrideBody    map[string]any
	StatusCode      int
	Headers         map[string]string
	Body            map[string]any
}

type Stats struct {
	Avg         time.Duration
	High        time.Duration
	Low         time.Duration
	SuccessRate float64
}

func (c *Client) RunEndpointN(endpoint *Endpoint, n int, workers int) (*Stats, error) {
	if endpoint == nil ||
		n <= 0 ||
		workers <= 0 ||
		endpoint.Path == "" ||
		endpoint.Method == "" {
		return nil, fmt.Errorf("invalid parameters")
	}

	if workers > n {
		workers = n
	}

	testcases, err := c.CreateTestcasesFromEndpoint(endpoint)
	if err != nil {
		return nil, err
	}

	// Generator
	testcasesCh := make(chan *testcase)
	go func() {
		defer close(testcasesCh)
		for i := range n {
			index := i % len(testcases)
			testcasesCh <- testcases[index]
		}
	}()

	// Fan-out
	var wg sync.WaitGroup
	resultsCh := make(chan time.Duration)

	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for tc := range testcasesCh {
				dur, err := c.testcase(tc)
				if err != nil {
					fmt.Println("testcase error:", err)
					continue
				}
				resultsCh <- dur
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	// Fan-in
	count := 0
	var avg, high, low time.Duration
	low = time.Hour

	for dur := range resultsCh {
		count++
		avg += dur
		high = max(high, dur)
		low = min(low, dur)
	}

	if count > 0 {
		avg /= time.Duration(count)
	}

	rate := float64(count) / float64(n)

	return &Stats{
		Avg:         avg,
		High:        high,
		Low:         low,
		SuccessRate: rate,
	}, nil
}
