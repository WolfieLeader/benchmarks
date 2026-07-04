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
				SuccessRate: 1,
			},
			Open: &client.OpenStats{
				TargetRate: 100, ScheduleLagP99: 2 * time.Millisecond,
				Response: &client.Stats{Avg: 7 * time.Millisecond},
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
	// strings. (EndpointResult stats are converted to StatsSummary *_ns fields;
	// rps/open plumbing into summaries is the follow-up reporting slice.)
	for _, want := range []string{
		`"avg_duration": 25000000`,
		`"avg": 5000000`,
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
}
