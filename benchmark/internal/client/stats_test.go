package client

import (
	"testing"
	"time"
)

// Percentile uses linear interpolation (percentile_cont / R type-7). The
// canonical hand-computed case is [10,20,30,40] p50: rank = 0.5·(4-1) = 1.5,
// so the result is halfway between sorted[1]=20 and sorted[2]=30 → 25.
func TestPercentileInterpolation(t *testing.T) {
	t.Parallel()

	ns := func(v int64) time.Duration { return time.Duration(v) }

	cases := []struct {
		name   string
		sorted []time.Duration
		p      float64
		want   time.Duration
	}{
		{"even p50 interpolates", []time.Duration{ns(10), ns(20), ns(30), ns(40)}, 50, ns(25)},
		{"even p25 interpolates", []time.Duration{ns(10), ns(20), ns(30), ns(40)}, 25, ns(17)}, // rank 0.75 → 10 + 0.75·10
		{"even p75 interpolates", []time.Duration{ns(10), ns(20), ns(30), ns(40)}, 75, ns(32)}, // rank 2.25 → 30 + 0.25·10
		{"p0 is min", []time.Duration{ns(10), ns(20), ns(30), ns(40)}, 0, ns(10)},
		{"p100 is max", []time.Duration{ns(10), ns(20), ns(30), ns(40)}, 100, ns(40)},
		{"exact rank lands on element", []time.Duration{ns(0), ns(100), ns(200)}, 50, ns(100)},   // rank 1.0
		{"p99.9 near the top", []time.Duration{ns(0), ns(100), ns(200), ns(300)}, 99.9, ns(299)}, // rank 2.997 → 200 + 0.997·100
		{"single element any p", []time.Duration{ns(42)}, 50, ns(42)},
		{"single element p0", []time.Duration{ns(42)}, 0, ns(42)},
		{"single element p100", []time.Duration{ns(42)}, 100, ns(42)},
		{"empty is zero", nil, 50, 0},
		{"negative p clamps to min", []time.Duration{ns(5), ns(9)}, -1, ns(5)},
		{"over 100 clamps to max", []time.Duration{ns(5), ns(9)}, 150, ns(9)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := Percentile(tc.sorted, tc.p); got != tc.want {
				t.Errorf("Percentile(%v, %v) = %d, want %d", tc.sorted, tc.p, got, tc.want)
			}
		})
	}
}

// CalculateStats must populate P999 alongside the other percentiles and sort
// its input in place, so a shuffled slice still yields correct percentiles.
func TestCalculateStatsPopulatesP999(t *testing.T) {
	t.Parallel()

	latencies := make([]time.Duration, 0, 1000)
	for i := 1000; i >= 1; i-- { // descending: forces the in-place sort to matter
		latencies = append(latencies, time.Duration(i)*time.Millisecond)
	}

	stats := CalculateStats(latencies, len(latencies), len(latencies), time.Second)

	// rank(99.9) = 0.999·999 = 998.001 → between sorted[998]=999ms and
	// sorted[999]=1000ms: 999ms + 0.001·1ms = 999.001ms → truncates to 999ms + 1000ns.
	want := 999*time.Millisecond + time.Duration(0.001*float64(time.Millisecond))
	if stats.P999 != want {
		t.Errorf("P999 = %v, want %v", stats.P999, want)
	}
	if !(stats.P50 < stats.P95 && stats.P95 < stats.P99 && stats.P99 < stats.P999) {
		t.Errorf("percentiles not monotonic: p50=%v p95=%v p99=%v p999=%v",
			stats.P50, stats.P95, stats.P99, stats.P999)
	}
	if stats.Low != time.Millisecond || stats.High != time.Second {
		t.Errorf("low/high = %v/%v, want 1ms/1s", stats.Low, stats.High)
	}
}
