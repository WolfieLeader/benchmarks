package client

import (
	"slices"
	"time"
)

type Stats struct {
	Count       int           `json:"count"`
	TotalCount  int           `json:"total_count"`
	Avg         time.Duration `json:"avg"`
	High        time.Duration `json:"high"`
	Low         time.Duration `json:"low"`
	P50         time.Duration `json:"p50"`
	P95         time.Duration `json:"p95"`
	P99         time.Duration `json:"p99"`
	SuccessRate float64       `json:"success_rate"`
}

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
		Count:       successCount,
		TotalCount:  totalCount,
		Avg:         total / time.Duration(len(latencies)),
		Low:         low,
		High:        high,
		P50:         Percentile(latencies, 50),
		P95:         Percentile(latencies, 95),
		P99:         Percentile(latencies, 99),
		SuccessRate: successRate,
	}
}

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
	index := (p * (len(sorted) - 1)) / 100
	return sorted[index]
}
