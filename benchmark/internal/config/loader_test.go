package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// LoadTarget must resolve a config without any roster on disk — target mode
// benchmarks an externally-managed server (calibration gate, PLAN §7.6).
func TestLoadTarget(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "calibration.json")
	cfgJSON := `{
		"benchmark": {
			"concurrency": 8,
			"duration_per_endpoint": "1s",
			"request_timeout": "2s",
			"load": { "mode": "open", "rate": 100 }
		},
		"databases": [],
		"endpoints": {
			"health": { "route": "GET /health", "expect": { "status": 200, "text": "OK" } }
		}
	}`
	if err := os.WriteFile(path, []byte(cfgJSON), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, target, err := LoadTarget(path)
	if err != nil {
		t.Fatalf("LoadTarget: %v", err)
	}
	if target.Name != "target" {
		t.Errorf("target name: got %q, want %q", target.Name, "target")
	}
	if cfg.Benchmark.Concurrency != 8 {
		t.Errorf("concurrency: got %d, want 8", cfg.Benchmark.Concurrency)
	}
	if target.Load.Mode != LoadModeOpen || target.Load.Rate != 100 || target.Load.MaxInFlight != DefaultMaxInFlight {
		t.Errorf("load config not threaded: %+v", target.Load)
	}
	if len(target.Testcases) != 1 || target.Testcases[0].Method != "GET" || target.Testcases[0].Path != "/health" {
		t.Fatalf("testcases not resolved: %+v", target.Testcases)
	}
}

func TestApplyLoadDefaults(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		load    LoadConfig
		wantErr string
		check   func(t *testing.T, load LoadConfig)
	}{
		{
			name: "empty defaults to closed",
			load: LoadConfig{},
			check: func(t *testing.T, load LoadConfig) {
				if load.Mode != LoadModeClosed {
					t.Errorf("mode: got %q, want %q", load.Mode, LoadModeClosed)
				}
			},
		},
		{
			name:    "closed rejects open-mode knobs",
			load:    LoadConfig{Mode: LoadModeClosed, Rate: 100},
			wantErr: `require mode "open"`,
		},
		{
			name: "open constant rate applies max_in_flight default",
			load: LoadConfig{Mode: LoadModeOpen, Rate: 500},
			check: func(t *testing.T, load LoadConfig) {
				if load.MaxInFlight != DefaultMaxInFlight {
					t.Errorf("max_in_flight: got %d, want %d", load.MaxInFlight, DefaultMaxInFlight)
				}
			},
		},
		{
			name:    "open requires rate or stages",
			load:    LoadConfig{Mode: LoadModeOpen},
			wantErr: "requires a rate or stages",
		},
		{
			name:    "open rejects negative rate",
			load:    LoadConfig{Mode: LoadModeOpen, Rate: -1},
			wantErr: "rate must be >= 0",
		},
		{
			name:    "open rejects max_in_flight above schema ceiling",
			load:    LoadConfig{Mode: LoadModeOpen, Rate: 100, MaxInFlight: MaxInFlightCeiling + 1},
			wantErr: "between 1 and 100000",
		},
		{
			name: "stages parse durations",
			load: LoadConfig{Mode: LoadModeOpen, Stages: []StageConfig{
				{Target: 100, DurationRaw: "30s"},
				{Target: 0, DurationRaw: "500ms"},
			}},
			check: func(t *testing.T, load LoadConfig) {
				if load.Stages[0].Duration != 30*time.Second || load.Stages[1].Duration != 500*time.Millisecond {
					t.Errorf("stage durations not parsed: %v, %v", load.Stages[0].Duration, load.Stages[1].Duration)
				}
			},
		},
		{
			name:    "stage rejects bad duration",
			load:    LoadConfig{Mode: LoadModeOpen, Stages: []StageConfig{{Target: 10, DurationRaw: "fast"}}},
			wantErr: "stage 1 duration",
		},
		{
			name:    "stage rejects missing duration",
			load:    LoadConfig{Mode: LoadModeOpen, Stages: []StageConfig{{Target: 10}}},
			wantErr: "stage 1 duration",
		},
		{
			name:    "stage rejects negative target",
			load:    LoadConfig{Mode: LoadModeOpen, Stages: []StageConfig{{Target: -5, DurationRaw: "1s"}}},
			wantErr: "stage 1 target must be >= 0",
		},
		{
			name:    "open rejects all-zero rate curve",
			load:    LoadConfig{Mode: LoadModeOpen, Stages: []StageConfig{{Target: 0, DurationRaw: "1s"}}},
			wantErr: "positive rate or stage target",
		},
		{
			name:    "unknown mode rejected",
			load:    LoadConfig{Mode: "burst"},
			wantErr: `must be "closed" or "open"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			load := tc.load
			err := applyLoadDefaults(&load)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error: got %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.check != nil {
				tc.check(t, load)
			}
		})
	}
}
