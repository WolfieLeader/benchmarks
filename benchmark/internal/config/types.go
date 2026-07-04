package config

import "time"

// Config holds benchmark parameters only. The server roster is NOT here — it is
// discovered from servers/*/bench.json manifests (PLAN §7.4, internal/roster).
type Config struct {
	Benchmark     BenchmarkConfig           `json:"benchmark"`
	Container     ContainerConfig           `json:"container"`
	Databases     []string                  `json:"databases"`
	Endpoints     map[string]EndpointConfig `json:"endpoints"`
	EndpointOrder []string                  `json:"-"`
}

type BenchmarkConfig struct {
	BaseUrl                string     `json:"base_url"`
	Concurrency            int        `json:"concurrency"`
	DurationPerEndpointRaw string     `json:"duration_per_endpoint"`
	RequestTimeoutRaw      string     `json:"request_timeout"`
	SampleRateRaw          string     `json:"sample_rate,omitempty"`
	ServerCooldownRaw      string     `json:"server_cooldown,omitempty"`
	WarmupDurationRaw      string     `json:"warmup_duration,omitempty"`
	WarmupPauseRaw         string     `json:"warmup_pause,omitempty"`
	Load                   LoadConfig `json:"load,omitzero"`

	DurationPerEndpoint time.Duration `json:"-"`
	RequestTimeout      time.Duration `json:"-"`
	SampleRatePct       float64       `json:"-"`
	ServerCooldown      time.Duration `json:"-"`
	WarmupDuration      time.Duration `json:"-"`
	WarmupPause         time.Duration `json:"-"`
}

// LoadConfig selects the load model (PLAN §7.1). "closed" (default) is the
// existing worker loop where Concurrency workers issue requests back-to-back.
// "open" schedules requests on a constant-arrival-rate timetable so a slow
// server cannot suppress the samples that would record its stalls
// (coordinated omission); saturation surfaces as schedule lag, backlog, and
// dropped iterations instead of silently degrading into a closed loop.
// Sequences always run the closed loop; open mode applies to endpoint runs.
type LoadConfig struct {
	Mode string `json:"mode,omitempty"` // "closed" (default) or "open"
	// Rate is the arrival rate in requests/sec. Without stages it is constant
	// for duration_per_endpoint; with stages it is the starting rate.
	Rate float64 `json:"rate,omitempty"`
	// Stages ramp the arrival rate linearly from the previous rate (initially
	// Rate) to each stage's target over its duration (k6 ramping-arrival-rate
	// semantics). When present, the endpoint window is the stages' total
	// duration, not duration_per_endpoint.
	Stages []StageConfig `json:"stages,omitempty"`
	// MaxInFlight caps concurrent in-flight requests (default 512); an
	// equal-sized backlog queue absorbs bursts, and arrivals beyond both are
	// counted as dropped iterations — the arrival clock never blocks.
	MaxInFlight int `json:"max_in_flight,omitempty"`
}

type StageConfig struct {
	Target      float64 `json:"target"`   // arrival rate at the end of the stage (req/sec)
	DurationRaw string  `json:"duration"` // stage length, e.g. "30s"

	Duration time.Duration `json:"-"`
}

type ContainerConfig struct {
	CpuLimit    float64 `json:"cpu_limit"`
	MemoryLimit string  `json:"memory_limit"`
}

type EndpointConfig struct {
	Route       string            `json:"route,omitempty"`  // "GET /path" shorthand
	Path        string            `json:"path,omitempty"`   // parsed from route or explicit
	Method      string            `json:"method,omitempty"` // parsed from route or explicit
	Headers     map[string]string `json:"headers,omitempty"`
	Query       map[string]string `json:"query,omitempty"`
	Body        any               `json:"body,omitempty"`
	FormData    map[string]string `json:"form_data,omitempty"`
	File        string            `json:"file,omitempty"`
	Expect      ExpectConfig      `json:"expect"`
	PerDatabase bool              `json:"per_database,omitempty"`
	Variations  []VariationConfig `json:"variations,omitempty"`
	Sequence    *SequenceConfig   `json:"sequence,omitempty"`
}

type ExpectConfig struct {
	Status  int               `json:"status,omitempty"`
	Body    any               `json:"body,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Text    string            `json:"text,omitempty"`
}

type VariationConfig struct {
	Path     string            `json:"path,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	Query    map[string]string `json:"query,omitempty"`
	Body     any               `json:"body,omitempty"`
	FormData map[string]string `json:"form_data,omitempty"`
	File     string            `json:"file,omitempty"`
	Expect   *ExpectConfig     `json:"expect,omitempty"`
}

type SequenceConfig struct {
	Id      string               `json:"id"`
	Capture map[string]string    `json:"capture,omitempty"` // {"id": "id"} = capture response.id as {id}
	Vars    map[string]VarConfig `json:"vars,omitempty"`    // variable definitions (only on first endpoint)
}

type VarConfig struct {
	Type     string `json:"type"`               // "email" or "int"
	Min      int    `json:"min,omitempty"`      // for int type
	Max      int    `json:"max,omitempty"`      // for int type
	Optional any    `json:"optional,omitempty"` // true, false, or float 0.0-1.0
}

type ResolvedSequence struct {
	Id        string
	Database  string // empty if not per_database
	Vars      map[string]VarConfig
	Endpoints []*ResolvedSequenceEndpoint
}

type ResolvedSequenceEndpoint struct {
	Name           string
	Method         string
	Path           string // with {database} replaced, but {id} etc preserved
	Body           any
	Headers        map[string]string
	ExpectedStatus int
	ExpectedBody   any
	Capture        map[string]string
}
