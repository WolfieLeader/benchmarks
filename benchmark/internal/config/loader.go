package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Load reads and parses a configuration file, auto-detecting the format
func Load(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(filename))

	if ext != ".json" {
		return nil, fmt.Errorf("unsupported config file format: %s", ext)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse JSON config: %w", err)
	}

	applyDefaults(&cfg)

	return &cfg, nil
}

func applyDefaults(cfg *Config) {
	// Global defaults
	if cfg.Global.BaseURL == "" {
		cfg.Global.BaseURL = "http://localhost:8080"
	}
	if cfg.Global.Timeout == "" {
		cfg.Global.Timeout = "10s"
	}
	if cfg.Global.Workers == 0 {
		cfg.Global.Workers = 100
	}
	if cfg.Global.Iterations == 0 {
		cfg.Global.Iterations = 1000
	}

	// Endpoint defaults
	for name, endpoint := range cfg.Endpoints {
		if endpoint.Method == "" {
			endpoint.Method = "GET"
		}
		if endpoint.ExpectedStatus == 0 {
			endpoint.ExpectedStatus = 200
		}
		cfg.Endpoints[name] = endpoint
	}

	// Server defaults
	for name, server := range cfg.Servers {
		if server.Port == 0 {
			server.Port = 8080
		}
		cfg.Servers[name] = server
	}
}
