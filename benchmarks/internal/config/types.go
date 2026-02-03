package config

import "time"

type Config struct {
	Benchmark     BenchmarkConfig           `json:"benchmark"`
	Container     ContainerConfig           `json:"container"`
	Capacity      CapacityConfig            `json:"capacity"`
	Influx        InfluxConfig              `json:"influx"`
	Databases     []string                  `json:"databases"`
	Servers       []ServerConfig            `json:"servers"`
	Endpoints     map[string]EndpointConfig `json:"endpoints"`
	EndpointOrder []string                  `json:"-"`
}

type InfluxConfig struct {
	URL           string  `json:"url"`
	Database      string  `json:"database"`
	Token         string  `json:"token"`
	SampleRate    string  `json:"sample_rate"`
	SampleRatePct float64 `json:"-"`
}

type BenchmarkConfig struct {
	BaseURL             string `json:"base_url"`
	Concurrency         int    `json:"concurrency"`
	RequestsPerEndpoint int    `json:"requests_per_endpoint"`
	RequestTimeoutRaw   string `json:"request_timeout"`
	ServerCooldownRaw   string `json:"server_cooldown,omitempty"`
	WarmupDurationRaw   string `json:"warmup_duration,omitempty"`
	WarmupPauseRaw      string `json:"warmup_pause,omitempty"`

	RequestTimeout time.Duration `json:"-"`
	ServerCooldown time.Duration `json:"-"`
	WarmupDuration time.Duration `json:"-"`
	WarmupPause    time.Duration `json:"-"`

	WarmupEnabled    bool `json:"-"`
	ResourcesEnabled bool `json:"-"`
}

type ContainerConfig struct {
	CPULimit    float64 `json:"cpu_limit"`
	MemoryLimit string  `json:"memory_limit"`
}

type CapacityConfig struct {
	Enabled             bool   `json:"-"`
	MinConcurrency      int    `json:"min_concurrency"`
	MaxConcurrency      int    `json:"max_concurrency"`
	SearchPrecision     string `json:"search_precision"`
	MinSuccessRate      string `json:"min_success_rate"`
	P99LatencyThreshold string `json:"p99_latency_threshold"`
	PreRunPauseRaw      string `json:"pre_run_pause"`
	WarmupDurationRaw   string `json:"warmup_duration"`
	MeasureDurationRaw  string `json:"measure_duration"`
	IterationPauseRaw   string `json:"iteration_pause"`

	SearchPrecisionPct     float64       `json:"-"`
	MinSuccessRatePct      float64       `json:"-"`
	P99LatencyThresholdDur time.Duration `json:"-"`
	PreRunPause            time.Duration `json:"-"`
	WarmupDuration         time.Duration `json:"-"`
	MeasureDuration        time.Duration `json:"-"`
	IterationPause         time.Duration `json:"-"`
}

type ServerConfig struct {
	Name  string `json:"name"`
	Image string `json:"image"`
	Port  int    `json:"port"`
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
	Flow        *FlowConfig       `json:"flow,omitempty"`
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

type FlowConfig struct {
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

type ResolvedFlow struct {
	Id        string
	Database  string // empty if not per_database
	Vars      map[string]VarConfig
	Endpoints []*ResolvedFlowEndpoint
}

type ResolvedFlowEndpoint struct {
	Name           string
	Method         string
	Path           string // with {database} replaced, but {id} etc preserved
	Body           any
	Headers        map[string]string
	ExpectedStatus int
	ExpectedBody   any
	Capture        map[string]string
}
