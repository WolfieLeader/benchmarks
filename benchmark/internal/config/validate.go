package config

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
)

var validMethods = []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}

// Validate validates the entire configuration
func Validate(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("configuration is nil")
	}

	if err := validateGlobal(&cfg.Global); err != nil {
		return fmt.Errorf("global config: %w", err)
	}

	if len(cfg.Endpoints) == 0 {
		return fmt.Errorf("no endpoints defined")
	}

	for name, endpoint := range cfg.Endpoints {
		if err := validateEndpoint(name, &endpoint); err != nil {
			return fmt.Errorf("endpoint %q: %w", name, err)
		}
	}

	if len(cfg.Servers) == 0 {
		return fmt.Errorf("no servers defined")
	}

	for name, server := range cfg.Servers {
		if err := validateServer(name, &server); err != nil {
			return fmt.Errorf("server %q: %w", name, err)
		}
	}

	return nil
}

func validateGlobal(g *GlobalConfig) error {
	if g.BaseURL == "" {
		return fmt.Errorf("base_url is required")
	}

	if _, err := url.Parse(g.BaseURL); err != nil {
		return fmt.Errorf("invalid base_url: %w", err)
	}

	if g.Timeout != "" {
		if _, err := time.ParseDuration(g.Timeout); err != nil {
			return fmt.Errorf("invalid timeout format: %w", err)
		}
	}

	if g.Workers < 0 {
		return fmt.Errorf("workers must be non-negative")
	}

	if g.Iterations < 0 {
		return fmt.Errorf("iterations must be non-negative")
	}

	return nil
}

func validateEndpoint(name string, e *EndpointConfig) error {
	if name == "" {
		return fmt.Errorf("endpoint name is required")
	}

	if e.Path == "" {
		return fmt.Errorf("path is required")
	}

	if !strings.HasPrefix(e.Path, "/") {
		return fmt.Errorf("path must start with /")
	}

	method := strings.ToUpper(e.Method)
	if !slices.Contains(validMethods, method) {
		return fmt.Errorf("invalid method %q", e.Method)
	}

	if e.ExpectedStatus < 100 || e.ExpectedStatus > 599 {
		return fmt.Errorf("expected_status must be between 100 and 599")
	}

	// Validate that at least one response expectation is set
	if e.ExpectedBody == nil && e.ExpectedText == "" {
		return fmt.Errorf("expected_body or expected_text is required")
	}

	// Validate test cases
	for i, tc := range e.TestCases {
		if err := validateTestCase(i, &tc, e); err != nil {
			return fmt.Errorf("test_case[%d]: %w", i, err)
		}
	}

	// Validate file config if present
	if e.File != nil {
		if err := validateFileConfig(e.File); err != nil {
			return fmt.Errorf("file: %w", err)
		}
	}

	return nil
}

func validateTestCase(index int, tc *TestCaseConfig, endpoint *EndpointConfig) error {
	if tc.Name == "" {
		return fmt.Errorf("name is required")
	}

	// Path override must be valid if specified
	if tc.Path != "" && !strings.HasPrefix(tc.Path, "/") {
		return fmt.Errorf("path override must start with /")
	}

	// Status code override must be valid if specified
	if tc.ExpectedStatus != 0 && (tc.ExpectedStatus < 100 || tc.ExpectedStatus > 599) {
		return fmt.Errorf("expected_status override must be between 100 and 599")
	}

	// Validate file config if present
	if tc.File != nil {
		if err := validateFileConfig(tc.File); err != nil {
			return fmt.Errorf("file: %w", err)
		}
	}

	return nil
}

func validateFileConfig(f *FileConfig) error {
	if f.FieldName == "" {
		return fmt.Errorf("field_name is required")
	}
	if f.Filename == "" {
		return fmt.Errorf("filename is required")
	}
	// Content can be empty for testing empty files
	return nil
}

func validateServer(name string, s *ServerConfig) error {
	if name == "" {
		return fmt.Errorf("server name is required")
	}

	if s.ImageName == "" {
		return fmt.Errorf("image_name is required")
	}

	if s.Port < 0 || s.Port > 65535 {
		return fmt.Errorf("port must be between 0 and 65535")
	}

	// Validate container limits if specified
	if s.Container.CPULimit != "" {
		if err := validateCPULimit(s.Container.CPULimit); err != nil {
			return fmt.Errorf("container.cpu_limit: %w", err)
		}
	}

	if s.Container.MemoryLimit != "" {
		if err := validateMemoryLimit(s.Container.MemoryLimit); err != nil {
			return fmt.Errorf("container.memory_limit: %w", err)
		}
	}

	// Validate overrides if present
	if s.Overrides != nil {
		if s.Overrides.Workers != nil && *s.Overrides.Workers < 0 {
			return fmt.Errorf("overrides.workers must be non-negative")
		}
		if s.Overrides.Iterations != nil && *s.Overrides.Iterations < 0 {
			return fmt.Errorf("overrides.iterations must be non-negative")
		}
	}

	return nil
}

func validateCPULimit(limit string) error {
	// CPU limit can be a decimal number (e.g., "1.5" for 1.5 CPUs)
	// or a percentage (e.g., "50%")
	// For simplicity, just check it's not empty - Docker will validate the actual format
	if limit == "" {
		return fmt.Errorf("cpu_limit is empty")
	}
	return nil
}

func validateMemoryLimit(limit string) error {
	// Memory limit should be in format like "512m", "1g", "256k"
	// Docker will validate the actual format
	if limit == "" {
		return fmt.Errorf("memory_limit is empty")
	}
	return nil
}
