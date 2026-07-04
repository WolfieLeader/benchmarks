package config

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"benchmark-client/internal/roster"
)

var DefaultConfig = Config{
	Benchmark: BenchmarkConfig{
		BaseUrl:                "http://localhost:8080",
		Concurrency:            50,
		DurationPerEndpointRaw: "10s",
		RequestTimeoutRaw:      "10s",
		SampleRateRaw:          "10%",
		WarmupDurationRaw:      "1s",
		WarmupPauseRaw:         "100ms",
	},
	Container: ContainerConfig{
		CpuLimit:    1.0,
		MemoryLimit: "512mb",
	},
}

const (
	DefaultConfigFile = "../config/config.json"
	DefaultMethod     = "GET"
	DefaultStatus     = 200

	LoadModeClosed = "closed"
	LoadModeOpen   = "open"

	DefaultMaxInFlight = 512
	// MaxInFlightCeiling mirrors the JSON schema's maximum — the schema is
	// editor-only until runtime validation lands, so the loader enforces it.
	MaxInFlightCeiling = 100000
)

var validMethods = []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}

// Load reads benchmark parameters from filename and discovers the server roster
// from serversDir (servers/*/bench.json manifests, PLAN §7.4). The roster no
// longer lives in the config file.
func Load(filename, serversDir string) (*Config, []*ResolvedServer, error) {
	cfg, err := loadConfigFile(filename)
	if err != nil {
		return nil, nil, err
	}

	entries, err := roster.Discover(serversDir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to discover server roster: %w", err)
	}

	resolved, err := resolve(cfg, entries)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve configuration: %w", err)
	}

	return cfg, resolved, nil
}

// LoadTarget reads benchmark parameters from filename for --target mode: one
// externally-managed server whose lifecycle the caller owns, so no roster
// discovery and no container metadata — the base URL is supplied at run time.
func LoadTarget(filename string) (*Config, *ResolvedServer, error) {
	cfg, err := loadConfigFile(filename)
	if err != nil {
		return nil, nil, err
	}

	resolved, err := resolve(cfg, []roster.Entry{{Name: "target"}})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve configuration: %w", err)
	}

	return cfg, resolved[0], nil
}

func loadConfigFile(filename string) (*Config, error) {
	data, err := os.ReadFile(filename) //nolint:gosec // config file path is controlled
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".json" {
		return nil, fmt.Errorf("unsupported config file format: %s", ext)
	}

	var cfg Config
	if err = json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse JSON config: %w", err)
	}

	order, err := extractKeyOrder(data, "endpoints")
	if err != nil {
		return nil, err
	}
	cfg.EndpointOrder = order

	if err = applyDefaults(&cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

func extractKeyOrder(data []byte, key string) ([]string, error) {
	// Deliberate: json/v2's default rejects duplicate top-level keys (v1 was
	// last-wins). Stricter-and-better for this trusted, repo-owned config file —
	// a duplicated key is a mistake we want surfaced, so no AllowDuplicateNames.
	var root map[string]jsontext.Value
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("failed to parse config for %s order: %w", key, err)
	}

	raw, ok := root[key]
	if !ok {
		return nil, nil
	}

	dec := jsontext.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.ReadToken()
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", key, err)
	}
	if tok.Kind() != '{' {
		return nil, fmt.Errorf("%s must be an object", key)
	}

	var order []string
	for dec.PeekKind() != '}' {
		keyTok, err := dec.ReadToken()
		if err != nil {
			return nil, fmt.Errorf("failed to read %s key: %w", key, err)
		}
		order = append(order, keyTok.String())
		if err := dec.SkipValue(); err != nil {
			return nil, fmt.Errorf("failed to skip %s value: %w", key, err)
		}
	}

	if _, err := dec.ReadToken(); err != nil {
		return nil, fmt.Errorf("failed to close %s object: %w", key, err)
	}

	return order, nil
}

func applyDefaults(cfg *Config) error {
	if cfg == nil {
		return errors.New("configuration is nil")
	}

	if strings.TrimSpace(cfg.Benchmark.BaseUrl) == "" {
		cfg.Benchmark.BaseUrl = DefaultConfig.Benchmark.BaseUrl
	}
	if _, err := url.Parse(cfg.Benchmark.BaseUrl); err != nil {
		return fmt.Errorf("benchmark base_url: %w", err)
	}

	if cfg.Benchmark.Concurrency <= 0 {
		cfg.Benchmark.Concurrency = DefaultConfig.Benchmark.Concurrency
	}

	var err error
	cfg.Benchmark.DurationPerEndpoint, err = validateDuration(
		&cfg.Benchmark.DurationPerEndpointRaw, DefaultConfig.Benchmark.DurationPerEndpointRaw,
		"benchmark duration_per_endpoint", false,
	)
	if err != nil {
		return err
	}

	cfg.Benchmark.RequestTimeout, err = validateDuration(
		&cfg.Benchmark.RequestTimeoutRaw, DefaultConfig.Benchmark.RequestTimeoutRaw,
		"benchmark request_timeout", false,
	)
	if err != nil {
		return err
	}

	defaultSampleRate, _ := parsePercent(DefaultConfig.Benchmark.SampleRateRaw, DefaultConfig.Benchmark.SampleRateRaw)
	sampleRate, err := parsePercent(cfg.Benchmark.SampleRateRaw, DefaultConfig.Benchmark.SampleRateRaw)
	if err != nil {
		return fmt.Errorf("benchmark sample_rate: %w", err)
	}
	if sampleRate <= 0 || sampleRate > 100 {
		sampleRate = defaultSampleRate
	}
	if strings.TrimSpace(cfg.Benchmark.SampleRateRaw) == "" {
		cfg.Benchmark.SampleRateRaw = DefaultConfig.Benchmark.SampleRateRaw
	}
	cfg.Benchmark.SampleRatePct = sampleRate / 100

	if strings.TrimSpace(cfg.Benchmark.ServerCooldownRaw) != "" {
		cooldown, cooldownErr := time.ParseDuration(cfg.Benchmark.ServerCooldownRaw)
		if cooldownErr != nil {
			return fmt.Errorf("benchmark server_cooldown: %w", cooldownErr)
		}
		if cooldown < 0 {
			return errors.New("benchmark server_cooldown must be >= 0")
		}
		cfg.Benchmark.ServerCooldown = cooldown
	}

	cfg.Benchmark.WarmupDuration, err = validateDuration(
		&cfg.Benchmark.WarmupDurationRaw, DefaultConfig.Benchmark.WarmupDurationRaw,
		"benchmark warmup_duration", true,
	)
	if err != nil {
		return err
	}

	cfg.Benchmark.WarmupPause, err = validateDuration(
		&cfg.Benchmark.WarmupPauseRaw, DefaultConfig.Benchmark.WarmupPauseRaw,
		"benchmark warmup_pause", true,
	)
	if err != nil {
		return err
	}

	err = applyLoadDefaults(&cfg.Benchmark.Load)
	if err != nil {
		return err
	}

	if cfg.Container.CpuLimit <= 0 {
		cfg.Container.CpuLimit = DefaultConfig.Container.CpuLimit
	}

	if strings.TrimSpace(cfg.Container.MemoryLimit) == "" {
		cfg.Container.MemoryLimit = DefaultConfig.Container.MemoryLimit
	}
	normalizedMemory, err := normalizeMemoryLimit(cfg.Container.MemoryLimit)
	if err != nil {
		return fmt.Errorf("container memory_limit: %w", err)
	}
	cfg.Container.MemoryLimit = normalizedMemory

	if len(cfg.Endpoints) == 0 {
		return errors.New("no endpoints defined")
	}

	for name := range cfg.Endpoints {
		endpoint := cfg.Endpoints[name]
		if err := applyEndpointDefaults(name, &endpoint); err != nil {
			return fmt.Errorf("endpoint %q: %w", name, err)
		}
		cfg.Endpoints[name] = endpoint
	}

	return nil
}

// applyLoadDefaults validates the load model selection. Closed mode must not
// carry open-mode knobs — a rate set under closed mode is an operator mistake
// we surface, not a silent no-op.
func applyLoadDefaults(load *LoadConfig) error {
	if strings.TrimSpace(load.Mode) == "" {
		load.Mode = LoadModeClosed
	}

	switch load.Mode {
	case LoadModeClosed:
		if load.Rate != 0 || len(load.Stages) != 0 || load.MaxInFlight != 0 {
			return errors.New(`benchmark load: rate, stages, and max_in_flight require mode "open"`)
		}
	case LoadModeOpen:
		if load.Rate < 0 {
			return errors.New("benchmark load rate must be >= 0")
		}
		if len(load.Stages) == 0 && load.Rate == 0 {
			return errors.New("benchmark load: open mode requires a rate or stages")
		}
		if load.MaxInFlight == 0 {
			load.MaxInFlight = DefaultMaxInFlight
		}
		if load.MaxInFlight < 1 || load.MaxInFlight > MaxInFlightCeiling {
			return fmt.Errorf("benchmark load max_in_flight must be between 1 and %d", MaxInFlightCeiling)
		}
		ratePositive := load.Rate > 0
		for i := range load.Stages {
			stage := &load.Stages[i]
			d, err := parseDuration(stage.DurationRaw, "")
			if err != nil {
				return fmt.Errorf("benchmark load stage %d duration: %w", i+1, err)
			}
			if d <= 0 {
				return fmt.Errorf("benchmark load stage %d duration must be > 0", i+1)
			}
			if stage.Target < 0 {
				return fmt.Errorf("benchmark load stage %d target must be >= 0", i+1)
			}
			if stage.Target > 0 {
				ratePositive = true
			}
			stage.Duration = d
		}
		if !ratePositive {
			return errors.New("benchmark load: open mode requires a positive rate or stage target")
		}
	default:
		return fmt.Errorf("benchmark load mode must be %q or %q, got %q", LoadModeClosed, LoadModeOpen, load.Mode)
	}

	return nil
}

// validateDuration parses a duration field, applies its default, and validates the result.
// When allowZero is false, the duration must be > 0; when true, it must be >= 0.
func validateDuration(raw *string, defaultRaw, fieldName string, allowZero bool) (time.Duration, error) {
	d, err := parseDuration(*raw, defaultRaw)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", fieldName, err)
	}
	if !allowZero && d <= 0 {
		return 0, fmt.Errorf("%s must be > 0", fieldName)
	}
	if allowZero && d < 0 {
		return 0, fmt.Errorf("%s must be >= 0", fieldName)
	}
	if strings.TrimSpace(*raw) == "" {
		*raw = defaultRaw
	}
	return d, nil
}

func applyEndpointDefaults(name string, e *EndpointConfig) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("endpoint name is required")
	}

	if route := strings.TrimSpace(e.Route); route != "" {
		parts := strings.SplitN(route, " ", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid route format %q, expected \"METHOD /path\"", route)
		}
		e.Method = strings.TrimSpace(parts[0])
		e.Path = strings.TrimSpace(parts[1])
	}

	if strings.TrimSpace(e.Path) == "" {
		return errors.New("path is required (use route or path field)")
	}
	if !strings.HasPrefix(e.Path, "/") {
		e.Path = "/" + e.Path
	}

	if strings.TrimSpace(e.Method) == "" {
		e.Method = DefaultMethod
	}
	e.Method = strings.ToUpper(strings.TrimSpace(e.Method))
	if !slices.Contains(validMethods, e.Method) {
		return fmt.Errorf("invalid method %q", e.Method)
	}

	if e.Expect.Status == 0 {
		e.Expect.Status = DefaultStatus
	}

	if e.Expect.Status < 100 || e.Expect.Status > 599 {
		return errors.New("expect.status must be between 100 and 599")
	}

	if e.Sequence != nil {
		if strings.TrimSpace(e.Sequence.Id) == "" {
			return errors.New("sequence.id is required when sequence is specified")
		}
		for varName, varCfg := range e.Sequence.Vars {
			if varCfg.Type != "email" && varCfg.Type != "int" {
				return fmt.Errorf("sequence.vars.%s: type must be \"email\" or \"int\"", varName)
			}
			if varCfg.Type == "int" && varCfg.Max < varCfg.Min {
				return fmt.Errorf("sequence.vars.%s: max must be >= min", varName)
			}
		}
	}

	return nil
}
