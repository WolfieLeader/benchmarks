package client

import (
	"slices"
	"time"
)

// Stats contains latency statistics for a set of requests.
type Stats struct {
	Avg         time.Duration `json:"avg"`
	High        time.Duration `json:"high"`
	Low         time.Duration `json:"low"`
	P50         time.Duration `json:"p50"`
	P95         time.Duration `json:"p95"`
	P99         time.Duration `json:"p99"`
	SuccessRate float64       `json:"success_rate"`
}

// CalculateStats computes statistics from a slice of latencies.
// The latencies slice will be sorted in place.
// successCount and totalCount are used to calculate success rate.
func CalculateStats(latencies []time.Duration, successCount, totalCount int) *Stats {
	if len(latencies) == 0 {
		return &Stats{}
	}

	var total time.Duration
	low := time.Hour
	var high time.Duration

	for _, l := range latencies {
		total += l
		if l < low {
			low = l
		}
		if l > high {
			high = l
		}
	}

	slices.Sort(latencies)

	var successRate float64
	if totalCount > 0 {
		successRate = float64(successCount) / float64(totalCount)
	}

	return &Stats{
		Avg:         total / time.Duration(len(latencies)),
		Low:         low,
		High:        high,
		P50:         Percentile(latencies, 50),
		P95:         Percentile(latencies, 95),
		P99:         Percentile(latencies, 99),
		SuccessRate: successRate,
	}
}

// Percentile calculates the p-th percentile from a sorted slice of durations.
func Percentile(sorted []time.Duration, p int) time.Duration {
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
