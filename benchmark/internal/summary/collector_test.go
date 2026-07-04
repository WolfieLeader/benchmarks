package summary

import (
	"encoding/json/v2"
	"os"
	"strings"
	"testing"
	"time"

	"benchmark-client/internal/client"
	"benchmark-client/internal/config"
)

// Reproduces the export failure found on the first full benchmark run after
// the json/v2 migration: ServerSummary embeds raw client.SequenceStats, whose
// time.Duration fields have no default json/v2 representation — Marshal
// errored with "cannot marshal from Go time.Duration" and the whole server
// result was silently lost. The `format:nano` tags keep the v1 int64-ns
// encoding.
func TestExportServerResultMarshalsDurations(t *testing.T) {
	t.Parallel()

	w := NewWriter(&config.BenchmarkConfig{}, t.TempDir())
	result := &ServerResult{
		Name: "test-server",
		Results: []client.EndpointResult{{
			Name:   "root",
			Path:   "/",
			Method: "GET",
			Stats: &client.Stats{
				Count: 1, TotalCount: 1, Rps: 10,
				Avg: 5 * time.Millisecond, Low: time.Millisecond, High: 9 * time.Millisecond,
				P50: 5 * time.Millisecond, P95: 8 * time.Millisecond, P99: 9 * time.Millisecond,
				P999:        9 * time.Millisecond,
				SuccessRate: 1,
			},
			Open: &client.OpenStats{
				TargetRate: 100, OfferedRate: 98, Attempted: 500, DroppedIterations: 3, MaxBacklog: 12,
				ScheduleLagP50: time.Millisecond, ScheduleLagP99: 2 * time.Millisecond, ScheduleLagMax: 4 * time.Millisecond,
				Response: &client.Stats{Avg: 7 * time.Millisecond, P99: 8 * time.Millisecond},
			},
		}},
		Sequences: []client.SequenceStats{{
			SequenceId: "crud", TotalRuns: 3, Successes: 3, SuccessRate: 1,
			AvgDuration: 25 * time.Millisecond,
			P50Duration: 24 * time.Millisecond,
			P95Duration: 30 * time.Millisecond,
			P99Duration: 31 * time.Millisecond,
			StepCount:   1,
			Steps: []client.SequenceStepStats{{
				Name: "create", Method: "POST", Path: "/db/postgres/users",
				Count: 3, Attempts: 3,
				Avg: 5 * time.Millisecond, Low: 4 * time.Millisecond, High: 6 * time.Millisecond,
				P50: 5 * time.Millisecond, P95: 6 * time.Millisecond, P99: 6 * time.Millisecond,
			}},
		}},
	}

	path, err := w.ExportServerResult(result)
	if err != nil {
		t.Fatalf("ExportServerResult: %v", err)
	}

	data, err := os.ReadFile(path) //nolint:gosec // path comes from t.TempDir
	if err != nil {
		t.Fatalf("read exported file: %v", err)
	}
	out := string(data)

	// Sequence durations are the raw client structs in the export — they must
	// serialize as int64 nanoseconds (v1-compatible), not error or render as
	// strings. The reporting slice adds RPS + p99.9 to StatsSummary and the
	// open-model block (schedule lag as *_ns, nested response stats).
	for _, want := range []string{
		`"avg_duration": 25000000`, // raw client.SequenceStats duration (custom marshaler)
		`"avg": 5000000`,           // raw client.SequenceStepStats duration
		`"rps": 10`,                // new: throughput plumbed into StatsSummary
		`"p999_ns": 9000000`,       // new: p99.9 percentile
		`"target_rate": 100`,       // new: open-model block
		`"offered_rate": 98`,
		`"dropped_iterations": 3`,
		`"max_backlog": 12`,
		`"schedule_lag_p99_ns": 2000000`, // schedule lag stored as *_ns int64
		`"schedule_lag_max_ns": 4000000`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("exported JSON missing %s\n%s", want, out)
		}
	}

	// Round-trip: the read-back path uses the matching custom unmarshaler.
	var back ServerSummary
	if err := json.Unmarshal(data, &back, durationOpts); err != nil {
		t.Fatalf("unmarshal exported file: %v", err)
	}
	if len(back.Sequences) != 1 || back.Sequences[0].AvgDuration != 25*time.Millisecond {
		t.Errorf("round-trip lost sequence durations: %+v", back.Sequences)
	}
	if len(back.Results) != 1 {
		t.Fatalf("round-trip lost endpoint results: %+v", back.Results)
	}
	ep := back.Results[0]
	if ep.Stats == nil || ep.Stats.Rps != 10 || ep.Stats.P999Ns != int64(9*time.Millisecond) {
		t.Errorf("round-trip lost stats rps/p999: %+v", ep.Stats)
	}
	if ep.Open == nil {
		t.Fatalf("round-trip lost open summary")
	}
	if ep.Open.TargetRate != 100 || ep.Open.OfferedRate != 98 ||
		ep.Open.DroppedIterations != 3 || ep.Open.MaxBacklog != 12 ||
		ep.Open.ScheduleLagP99Ns != int64(2*time.Millisecond) ||
		ep.Open.ScheduleLagMaxNs != int64(4*time.Millisecond) {
		t.Errorf("round-trip lost open fields: %+v", ep.Open)
	}
	if ep.Open.Response == nil || ep.Open.Response.AvgNs != int64(7*time.Millisecond) {
		t.Errorf("round-trip lost nested open response stats: %+v", ep.Open.Response)
	}
}
