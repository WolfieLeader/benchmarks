package client

import (
	"slices"
	"time"
)

type Stats struct {
	Count       int           `json:"count"`
	TotalCount  int           `json:"total_count"`
	Rps         float64       `json:"rps"` // completed requests (success + failure) per second of wall time
	Avg         time.Duration `json:"avg"`
	High        time.Duration `json:"high"`
	Low         time.Duration `json:"low"`
	P50         time.Duration `json:"p50"`
	P95         time.Duration `json:"p95"`
	P99         time.Duration `json:"p99"`
	SuccessRate float64       `json:"success_rate"`
}

// CalculateStats computes latency stats over the run's successful requests.
// elapsed is the run's wall-clock window (endpoint start to drain end) and
// drives throughput; latency fields stay zero when nothing succeeded, but
// counts, success rate, and RPS are still reported.
func CalculateStats(latencies []time.Duration, successCount, totalCount int, elapsed time.Duration) *Stats {
	stats := &Stats{
		Count:      successCount,
		TotalCount: totalCount,
	}
	if totalCount > 0 {
		stats.SuccessRate = float64(successCount) / float64(totalCount)
	}
	if sec := elapsed.Seconds(); sec > 0 {
		stats.Rps = float64(totalCount) / sec
	}

	if len(latencies) == 0 {
		return stats
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

	stats.Avg = total / time.Duration(len(latencies))
	stats.Low = low
	stats.High = high
	stats.P50 = Percentile(latencies, 50)
	stats.P95 = Percentile(latencies, 95)
	stats.P99 = Percentile(latencies, 99)
	return stats
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
