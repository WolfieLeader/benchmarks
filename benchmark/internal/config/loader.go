package config

import (
	"context"
	"encoding/json"
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
	DefaultConfigFile = "config.jsonc"
	DefaultPort       = 8080
	DefaultBaseURL    = "http://localhost:8080"
	DefaultWorkers    = 50
	DefaultIterations = 1000
	DefaultTimeout    = "10s"
	DefaultMethod     = "GET"
	DefaultStatus     = 200
	DefaultCPU        = "1"
	DefaultMemory     = "512M"
)

var validMethods = []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}

func Load(ctx context.Context, filename string) (*ResolvedConfig, error) {
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := checkCtx(ctx); err != nil {
		return nil, err
	}

	fileExtension := strings.ToLower(filepath.Ext(filename))
	if fileExtension != ".jsonc" && fileExtension != ".json" {
		return nil, fmt.Errorf("unsupported config file format: %s", fileExtension)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse JSON config: %w", err)
	}

	if err := checkCtx(ctx); err != nil {
		return nil, err
	}

	if err := applyDefaults(&cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	if err := checkCtx(ctx); err != nil {
		return nil, err
	}

	resolved, err := resolve(&cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve configuration: %w", err)
	}

	return resolved, nil
}

func applyDefaults(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("configuration is nil")
	}

	if err := applyGlobalDefaults(&cfg.Global); err != nil {
		return fmt.Errorf("global config: %w", err)
	}

	if len(cfg.Endpoints) == 0 {
		return fmt.Errorf("no endpoints defined")
	}

	for name, endpoint := range cfg.Endpoints {
		if err := applyEndpointDefaults(name, &endpoint); err != nil {
			return fmt.Errorf("endpoint %q: %w", name, err)
		}
		cfg.Endpoints[name] = endpoint
	}

	if len(cfg.Servers) == 0 {
		return fmt.Errorf("no servers defined")
	}

	for name, port := range cfg.Servers {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("image name is required")
		}

		if port == 0 {
			port = DefaultPort
		}
		if port < 0 || port > 65535 {
			return fmt.Errorf("port must be between 0 and 65535")
		}

		cfg.Servers[name] = port
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
	if err := validateMemory(g.MemoryLimit); err != nil {
		return fmt.Errorf("memory_limit: %w", err)
	}

	return nil
}

func applyEndpointDefaults(name string, e *EndpointConfig) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("endpoint name is required")
	}

	if strings.TrimSpace(e.Path) == "" {
		return fmt.Errorf("path is required")
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
		return fmt.Errorf("expected_status must be between 100 and 599")
	}

	if e.ExpectedBody == nil && e.ExpectedText == "" {
		return fmt.Errorf("expected_body or expected_text is required")
	}

	for i := range e.TestCases {
		if err := applyTestCaseDefaults(i, &e.TestCases[i]); err != nil {
			return fmt.Errorf("test_case[%d]: %w", i, err)
		}
	}

	if e.File != nil {
		if e.File.FieldName == "" || e.File.Filename == "" {
			return fmt.Errorf("file field_name and filename are required")
		}
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
		return fmt.Errorf("expected_status override must be between 100 and 599")
	}

	if tc.File != nil {
		if tc.File.FieldName == "" || tc.File.Filename == "" {
			return fmt.Errorf("file field_name and filename are required")
		}
	}

	return nil
}

func validateCpu(limit string) error {
	limit = strings.TrimSpace(limit)
	if limit == "" {
		return fmt.Errorf("cpu_limit is empty")
	}

	if strings.HasSuffix(limit, "%") {
		value := strings.TrimSuffix(limit, "%")
		if value == "" {
			return fmt.Errorf("cpu_limit must be a number or percentage")
		}
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil || parsed <= 0 {
			return fmt.Errorf("cpu_limit must be a number or percentage")
		}
		return nil
	}

	parsed, err := strconv.ParseFloat(limit, 64)
	if err != nil || parsed <= 0 {
		return fmt.Errorf("cpu_limit must be a number or percentage")
	}

	return nil
}

func validateMemory(limit string) error {
	limit = strings.TrimSpace(limit)
	if limit == "" {
		return fmt.Errorf("memory_limit is empty")
	}

	value := limit
	last := limit[len(limit)-1]
	if last < '0' || last > '9' {
		unit := strings.ToLower(string(last))
		switch unit {
		case "b", "k", "m", "g":
			value = limit[:len(limit)-1]
		default:
			return fmt.Errorf("memory_limit must be a number with optional unit (b/k/m/g)")
		}
	}

	if value == "" {
		return fmt.Errorf("memory_limit must be a number with optional unit (b/k/m/g)")
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed <= 0 {
		return fmt.Errorf("memory_limit must be a number with optional unit (b/k/m/g)")
	}

	return nil
}
