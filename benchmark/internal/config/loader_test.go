package config

import (
	"strings"
	"testing"
	"time"
)

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
