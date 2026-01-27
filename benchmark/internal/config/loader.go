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
	"strconv"
	"strings"
	"time"
)

const (
	DefaultConfigFile     = "config.json"
	DefaultPort           = 8080
	DefaultBaseURL        = "http://localhost:8080"
	DefaultWorkers        = 50
	DefaultIterations     = 1000
	DefaultTimeout        = "10s"
	DefaultMethod         = "GET"
	DefaultStatus         = 200
	DefaultCPU            = "1"
	DefaultMemory         = "512mb"
	DefaultWarmupRequests = 50

	DefaultCapacityMinWorkers   = 1
	DefaultCapacityMaxWorkers   = 512
	DefaultCapacityPrecision    = "5%"
	DefaultCapacitySuccessRate  = "95%"
	DefaultCapacityP99Threshold = "50ms"
	DefaultCapacityWarmup       = "2s"
	DefaultCapacityMeasure      = "10s"
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

	serverOrder, err := extractKeyOrder(data, "servers")
	if err != nil {
		return nil, nil, err
	}
	cfg.ServerOrder = serverOrder

	if err = applyDefaults(&cfg); err != nil {
		return nil, nil, fmt.Errorf("invalid configuration: %w", err)
	}

	resolved, err := resolve(&cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve configuration: %w", err)
	}

	return &cfg, resolved, nil
}

func applyDefaults(cfg *Config) error {
	if cfg == nil {
		return errors.New("configuration is nil")
	}

	if err := applyGlobalDefaults(&cfg.Global); err != nil {
		return fmt.Errorf("global config: %w", err)
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

	if len(cfg.Servers) == 0 {
		return errors.New("no servers defined")
	}

	for name, port := range cfg.Servers {
		if strings.TrimSpace(name) == "" {
			return errors.New("image name is required")
		}

		if port == 0 {
			port = DefaultPort
		}
		if port < 0 || port > 65535 {
			return errors.New("port must be between 0 and 65535")
		}

		cfg.Servers[name] = port
	}

	return nil
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
		return nil, fmt.Errorf("failed to read endpoints: %w", err)
	}

	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, errors.New("endpoints must be an object")
	}

	order := make([]string, 0)
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("failed to read endpoint key: %w", err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, errors.New("invalid endpoint key")
		}
		order = append(order, key)
		if err := skipJSONValue(dec); err != nil {
			return nil, err
		}
	}

	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("failed to close endpoints object: %w", err)
	}

	return order, nil
}

func skipJSONValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("failed to read endpoint value: %w", err)
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

func applyGlobalDefaults(g *GlobalConfig) error {
	if strings.TrimSpace(g.BaseURL) == "" {
		g.BaseURL = DefaultBaseURL
	}
	if _, err := url.Parse(g.BaseURL); err != nil {
		return fmt.Errorf("invalid base_url: %w", err)
	}

	if g.Workers <= 0 {
		g.Workers = DefaultWorkers
	}

	if g.RequestsPerEndpoint <= 0 {
		g.RequestsPerEndpoint = DefaultIterations
	}

	if strings.TrimSpace(g.Timeout) == "" {
		g.Timeout = DefaultTimeout
	}
	if _, err := time.ParseDuration(g.Timeout); err != nil {
		return fmt.Errorf("invalid timeout format: %w", err)
	}

	if strings.TrimSpace(g.CPULimit) == "" {
		g.CPULimit = DefaultCPU
	}
	if err := validateCpu(g.CPULimit); err != nil {
		return fmt.Errorf("cpu_limit: %w", err)
	}

	if strings.TrimSpace(g.MemoryLimit) == "" {
		g.MemoryLimit = DefaultMemory
	}
	normalizedMemory, err := normalizeMemoryLimit(g.MemoryLimit)
	if err != nil {
		return fmt.Errorf("memory_limit: %w", err)
	}
	g.MemoryLimit = normalizedMemory

	// Warmup defaults
	if g.Warmup.Enabled && g.Warmup.RequestsPerTestcase <= 0 {
		g.Warmup.RequestsPerTestcase = DefaultWarmupRequests
	}

	// Resources defaults (Docker stats API pushes ~1 sample/sec, no config needed)

	// Capacity defaults
	if g.Capacity.Enabled {
		if g.Capacity.MinWorkers <= 0 {
			g.Capacity.MinWorkers = DefaultCapacityMinWorkers
		}
		if g.Capacity.MaxWorkers <= 0 {
			g.Capacity.MaxWorkers = DefaultCapacityMaxWorkers
		}
		if g.Capacity.MaxWorkers < g.Capacity.MinWorkers {
			return fmt.Errorf("capacity max_workers (%d) must be >= min_workers (%d)", g.Capacity.MaxWorkers, g.Capacity.MinWorkers)
		}

		precision, err := parsePercent(g.Capacity.Precision, DefaultCapacityPrecision)
		if err != nil {
			return fmt.Errorf("capacity precision: %w", err)
		}
		if precision > 50 {
			return errors.New("capacity precision must be <= 50%%")
		}
		g.Capacity.PrecisionPct = precision

		successRate, err := parsePercent(g.Capacity.SuccessRate, DefaultCapacitySuccessRate)
		if err != nil {
			return fmt.Errorf("capacity success_rate: %w", err)
		}
		if successRate > 100 {
			return errors.New("capacity success_rate must be <= 100%%")
		}
		g.Capacity.SuccessRatePct = successRate / 100

		p99Threshold, err := parseDuration(g.Capacity.P99Threshold, DefaultCapacityP99Threshold)
		if err != nil {
			return fmt.Errorf("capacity p99_threshold: %w", err)
		}
		g.Capacity.P99ThresholdDur = p99Threshold

		warmup, err := parseDuration(g.Capacity.Warmup, DefaultCapacityWarmup)
		if err != nil {
			return fmt.Errorf("capacity warmup: %w", err)
		}
		g.Capacity.WarmupDuration = warmup

		measure, err := parseDuration(g.Capacity.Measure, DefaultCapacityMeasure)
		if err != nil {
			return fmt.Errorf("capacity measure: %w", err)
		}
		if measure <= 0 {
			return errors.New("capacity measure must be > 0")
		}
		g.Capacity.MeasureDuration = measure
	}

	if strings.TrimSpace(g.Cooldown) != "" {
		cooldown := strings.TrimSpace(g.Cooldown)
		parsed, err := time.ParseDuration(cooldown)
		if err != nil {
			return fmt.Errorf("cooldown: %w", err)
		}
		if parsed < 0 {
			return errors.New("cooldown must be >= 0")
		}
		if parsed == 0 {
			g.Cooldown = ""
		} else {
			g.Cooldown = parsed.String()
		}
	}

	return nil
}

func applyEndpointDefaults(name string, e *EndpointConfig) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("endpoint name is required")
	}

	if strings.TrimSpace(e.Path) == "" {
		return errors.New("path is required")
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

	if e.ExpectedStatus == 0 {
		e.ExpectedStatus = DefaultStatus
	}

	if e.ExpectedStatus < 100 || e.ExpectedStatus > 599 {
		return errors.New("expected_status must be between 100 and 599")
	}

	if e.ExpectedBody == nil && e.ExpectedText == "" {
		return errors.New("expected_body or expected_text is required")
	}

	for i := range e.TestCases {
		if err := applyTestCaseDefaults(i, &e.TestCases[i]); err != nil {
			return fmt.Errorf("test_case[%d]: %w", i, err)
		}
	}

	if strings.TrimSpace(e.File) == "" && e.File != "" {
		return errors.New("file filename is required")
	}

	return nil
}

func applyTestCaseDefaults(index int, tc *TestCaseConfig) error {
	if tc.Name == "" {
		tc.Name = fmt.Sprintf("test_case_%d", index)
	}

	if tc.Path != "" && !strings.HasPrefix(tc.Path, "/") {
		tc.Path = "/" + tc.Path
	}

	if tc.ExpectedStatus != 0 && (tc.ExpectedStatus < 100 || tc.ExpectedStatus > 599) {
		return errors.New("expected_status override must be between 100 and 599")
	}

	if strings.TrimSpace(tc.File) == "" && tc.File != "" {
		return errors.New("file filename is required")
	}

	return nil
}

func validateCpu(limit string) error {
	limit = strings.TrimSpace(limit)
	if limit == "" {
		return errors.New("cpu_limit is empty")
	}

	if value, ok := strings.CutSuffix(limit, "%"); ok {
		if value == "" {
			return errors.New("cpu_limit must be a number or percentage")
		}
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil || parsed <= 0 {
			return errors.New("cpu_limit must be a number or percentage")
		}
		return nil
	}

	parsed, err := strconv.ParseFloat(limit, 64)
	if err != nil || parsed <= 0 {
		return errors.New("cpu_limit must be a number or percentage")
	}

	return nil
}

func normalizeMemoryLimit(limit string) (string, error) {
	limit = strings.TrimSpace(limit)
	if limit == "" {
		return "", errors.New("memory_limit is empty")
	}

	lower := strings.ToLower(limit)
	idx := len(lower)
	for idx > 0 {
		ch := lower[idx-1]
		if ch >= 'a' && ch <= 'z' {
			idx--
			continue
		}
		break
	}

	value := strings.TrimSpace(lower[:idx])
	unit := strings.TrimSpace(lower[idx:])
	if value == "" {
		return "", errors.New("memory_limit must be a number with optional unit (k, m, g, kb, mb, gb)")
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed <= 0 {
		return "", errors.New("memory_limit must be a number with optional unit (k, m, g, kb, mb, gb)")
	}

	switch unit {
	case "":
		return value, nil
	case "k", "kb":
		return value + "kb", nil
	case "m", "mb":
		return value + "mb", nil
	case "g", "gb":
		return value + "gb", nil
	default:
		return "", errors.New("memory_limit must be a number with optional unit (k, m, g, kb, mb, gb)")
	}
}

func parsePercent(value, defaultValue string) (float64, error) {
	if strings.TrimSpace(value) == "" {
		value = defaultValue
	}
	value = strings.TrimSpace(value)
	if !strings.HasSuffix(value, "%") {
		return 0, fmt.Errorf("must be a percentage (e.g. %q)", defaultValue)
	}
	numStr := strings.TrimSuffix(value, "%")
	parsed, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("invalid percentage %q", value)
	}
	return parsed, nil
}

func parseDuration(value, defaultValue string) (time.Duration, error) {
	if strings.TrimSpace(value) == "" {
		value = defaultValue
	}
	return time.ParseDuration(strings.TrimSpace(value))
}
