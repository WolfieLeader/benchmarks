package client

import (
	"time"
)

const N = 1000

type LatencyStats struct {
	Avg                time.Duration
	High               time.Duration
	Low                time.Duration
	TotalRequests      int
	SuccessfulRequests int
}

func (c *Client) RunBenchmarks() *LatencyStats {
	durations := make([]time.Duration, 0, N)

	for range N {
		dur, ok := c.Run(rootEndpoint, 5*time.Second)
		if ok {
			durations = append(durations, dur)
		}
	}

	var avg, high, low time.Duration
	low = time.Hour // set to max possible

	for _, dur := range durations {
		avg += dur
		high = max(high, dur)
		low = min(low, dur)
	}
	if len(durations) > 0 {
		avg /= time.Duration(len(durations))
	}

	return &LatencyStats{Avg: avg, High: high, Low: low, TotalRequests: N, SuccessfulRequests: len(durations)}
}

var rootEndpoint = &Endpoint{
	Path:     "/",
	Method:   GET,
	Expected: newExpected(200, Headers{}, Body{"message": "Hello, World!"}),
}
