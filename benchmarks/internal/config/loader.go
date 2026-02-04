package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
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
	DefaultPort       = 8080
	DefaultMethod     = "GET"
	DefaultStatus     = 200
)

var validMethods = []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}

func Load(filename string) (*Config, []*ResolvedServer, error) {
	data, err := os.ReadFile(filename) //nolint:gosec // config file path is controlled
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read config file: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".json" {
		return nil, nil, fmt.Errorf("unsupported config file format: %s", ext)
	}

	var cfg Config
	if err = json.Unmarshal(data, &cfg); err != nil {
		return nil, nil, fmt.Errorf("failed to parse JSON config: %w", err)
	}

	order, err := extractKeyOrder(data, "endpoints")
	if err != nil {
		return nil, nil, err
	}
	cfg.EndpointOrder = order

	if err = applyDefaults(&cfg); err != nil {
		return nil, nil, fmt.Errorf("invalid configuration: %w", err)
	}

	resolved, err := resolve(&cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve configuration: %w", err)
	}

	return &cfg, resolved, nil
}

func extractKeyOrder(data []byte, key string) ([]string, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("failed to parse config for %s order: %w", key, err)
	}

	raw, ok := root[key]
	if !ok {
		return nil, nil
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", key, err)
	}

	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, fmt.Errorf("%s must be an object", key)
	}

	order := make([]string, 0)
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("failed to read %s key: %w", key, err)
		}
		k, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("invalid %s key", key)
		}
		order = append(order, k)
		if err := skipJSONValue(dec); err != nil {
			return nil, err
		}
	}

	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("failed to close %s object: %w", key, err)
	}

	return order, nil
}

func skipJSONValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("failed to read value: %w", err)
	}

	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}

	if delim != '{' && delim != '[' {
		return nil
	}

	depth := 1
	for depth > 0 {
		next, err := dec.Token()
		if err != nil {
			return fmt.Errorf("failed to skip endpoint value: %w", err)
		}
		if d, ok := next.(json.Delim); ok {
			switch d {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
		}
	}

	return nil
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

	durationPerEndpoint, err := parseDuration(cfg.Benchmark.DurationPerEndpointRaw, DefaultConfig.Benchmark.DurationPerEndpointRaw)
	if err != nil {
		return fmt.Errorf("benchmark duration_per_endpoint: %w", err)
	}
	if durationPerEndpoint <= 0 {
		return errors.New("benchmark duration_per_endpoint must be > 0")
	}
	if strings.TrimSpace(cfg.Benchmark.DurationPerEndpointRaw) == "" {
		cfg.Benchmark.DurationPerEndpointRaw = DefaultConfig.Benchmark.DurationPerEndpointRaw
	}
	cfg.Benchmark.DurationPerEndpoint = durationPerEndpoint

	if strings.TrimSpace(cfg.Benchmark.RequestTimeoutRaw) == "" {
		cfg.Benchmark.RequestTimeoutRaw = DefaultConfig.Benchmark.RequestTimeoutRaw
	}
	requestTimeout, err := time.ParseDuration(cfg.Benchmark.RequestTimeoutRaw)
	if err != nil {
		return fmt.Errorf("benchmark request_timeout: %w", err)
	}
	cfg.Benchmark.RequestTimeout = requestTimeout

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

	warmupDuration, err := parseDuration(cfg.Benchmark.WarmupDurationRaw, DefaultConfig.Benchmark.WarmupDurationRaw)
	if err != nil {
		return fmt.Errorf("benchmark warmup_duration: %w", err)
	}
	if warmupDuration < 0 {
		return errors.New("benchmark warmup_duration must be >= 0")
	}
	if strings.TrimSpace(cfg.Benchmark.WarmupDurationRaw) == "" {
		cfg.Benchmark.WarmupDurationRaw = DefaultConfig.Benchmark.WarmupDurationRaw
	}
	cfg.Benchmark.WarmupDuration = warmupDuration

	warmupPause, err := parseDuration(cfg.Benchmark.WarmupPauseRaw, DefaultConfig.Benchmark.WarmupPauseRaw)
	if err != nil {
		return fmt.Errorf("benchmark warmup_pause: %w", err)
	}
	if warmupPause < 0 {
		return errors.New("benchmark warmup_pause must be >= 0")
	}
	if strings.TrimSpace(cfg.Benchmark.WarmupPauseRaw) == "" {
		cfg.Benchmark.WarmupPauseRaw = DefaultConfig.Benchmark.WarmupPauseRaw
	}
	cfg.Benchmark.WarmupPause = warmupPause

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

	if len(cfg.Servers) == 0 {
		return errors.New("no servers defined")
	}

	for i, server := range cfg.Servers {
		if strings.TrimSpace(server.Name) == "" {
			return fmt.Errorf("server[%d]: name is required", i)
		}
		if strings.TrimSpace(server.Image) == "" {
			return fmt.Errorf("server[%d]: image is required", i)
		}
		if server.Port == 0 {
			cfg.Servers[i].Port = DefaultPort
		}
		if server.Port < 0 || server.Port > 65535 {
			return fmt.Errorf("server[%d]: port must be between 0 and 65535", i)
		}
	}

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
