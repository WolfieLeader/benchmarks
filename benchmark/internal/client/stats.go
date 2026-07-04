package client

import (
	"math"
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
	P999        time.Duration `json:"p999"`
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
	stats.P999 = Percentile(latencies, 99.9)
	return stats
}

// Percentile returns the p-th percentile (p in [0,100], fractional allowed —
// e.g. 99.9) of the already-sorted input, using linear interpolation between
// the two closest ranks (PostgreSQL percentile_cont / NIST "linear" / R type-7
// semantics). The caller must pass a slice sorted ascending; CalculateStats
// sorts before calling. For n samples the target rank is (p/100)·(n-1), so p0
// yields the min, p100 the max, and interior percentiles interpolate.
func Percentile(sorted []time.Duration, p float64) time.Duration {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[n-1]
	}
	rank := (p / 100) * float64(n-1)
	lo := int(math.Floor(rank))
	hi := lo + 1
	if hi >= n {
		return sorted[n-1]
	}
	frac := rank - float64(lo)
	return sorted[lo] + time.Duration(frac*float64(sorted[hi]-sorted[lo]))
}
