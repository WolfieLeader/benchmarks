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
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
	Org     string `json:"org"`
	Bucket  string `json:"bucket"`
	Token   string `json:"token"`
}

type BenchmarkConfig struct {
	BaseURL  string `json:"base_url"`
	Workers  int    `json:"workers"`
	Requests int    `json:"requests"`
	Timeout  string `json:"timeout"`
	Cooldown string `json:"cooldown,omitempty"`
	Warmup   int    `json:"warmup,omitempty"`

	TimeoutDuration  time.Duration `json:"-"`
	CooldownDuration time.Duration `json:"-"`

	WarmupEnabled    bool `json:"-"`
	ResourcesEnabled bool `json:"-"`
}

type ContainerConfig struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

type CapacityConfig struct {
	Enabled      bool   `json:"-"`
	MinWorkers   int    `json:"min_workers"`
	MaxWorkers   int    `json:"max_workers"`
	Precision    string `json:"precision"`
	SuccessRate  string `json:"success_rate"`
	P99Threshold string `json:"p99_threshold"`
	Warmup       string `json:"warmup"`
	Measure      string `json:"measure"`

	PrecisionPct    float64       `json:"-"`
	SuccessRatePct  float64       `json:"-"`
	P99ThresholdDur time.Duration `json:"-"`
	WarmupDuration  time.Duration `json:"-"`
	MeasureDuration time.Duration `json:"-"`
}

type ServerConfig struct {
	Name string `json:"name"`
	Port int    `json:"port"`
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
