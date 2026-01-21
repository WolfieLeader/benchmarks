package config

import "time"

// Config is the root configuration structure
type Config struct {
	Global    GlobalConfig              `json:"global" yaml:"global"`
	Endpoints map[string]EndpointConfig `json:"endpoints" yaml:"endpoints"`
	Servers   map[string]ServerConfig   `json:"servers" yaml:"servers"`
}

// GlobalConfig contains default settings applied to all tests
type GlobalConfig struct {
	BaseURL    string `json:"base_url" yaml:"base_url"`
	Timeout    string `json:"timeout" yaml:"timeout"`
	Workers    int    `json:"workers" yaml:"workers"`
	Iterations int    `json:"iterations" yaml:"iterations"`
}

// GetTimeout parses the timeout string and returns a duration
func (g *GlobalConfig) GetTimeout() time.Duration {
	if g.Timeout == "" {
		return 10 * time.Second
	}
	d, err := time.ParseDuration(g.Timeout)
	if err != nil {
		return 10 * time.Second
	}
	return d
}

// EndpointConfig defines an endpoint and its test cases
type EndpointConfig struct {
	Path            string            `json:"path" yaml:"path"`
	Method          string            `json:"method" yaml:"method"`
	Headers         map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	Query           map[string]string `json:"query,omitempty" yaml:"query,omitempty"`
	Body            any               `json:"body,omitempty" yaml:"body,omitempty"`
	FormData        map[string]string `json:"form_data,omitempty" yaml:"form_data,omitempty"`
	File            *FileConfig       `json:"file,omitempty" yaml:"file,omitempty"`
	ExpectedStatus  int               `json:"expected_status" yaml:"expected_status"`
	ExpectedHeaders map[string]string `json:"expected_headers,omitempty" yaml:"expected_headers,omitempty"`
	ExpectedBody    any               `json:"expected_body,omitempty" yaml:"expected_body,omitempty"`
	ExpectedText    string            `json:"expected_text,omitempty" yaml:"expected_text,omitempty"`
	TestCases       []TestCaseConfig  `json:"test_cases,omitempty" yaml:"test_cases,omitempty"`
}

// TestCaseConfig defines overrides for a specific test case
type TestCaseConfig struct {
	Name            string            `json:"name" yaml:"name"`
	Path            string            `json:"path,omitempty" yaml:"path,omitempty"`
	Headers         map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	Query           map[string]string `json:"query,omitempty" yaml:"query,omitempty"`
	Body            any               `json:"body,omitempty" yaml:"body,omitempty"`
	FormData        map[string]string `json:"form_data,omitempty" yaml:"form_data,omitempty"`
	File            *FileConfig       `json:"file,omitempty" yaml:"file,omitempty"`
	ExpectedStatus  int               `json:"expected_status,omitempty" yaml:"expected_status,omitempty"`
	ExpectedHeaders map[string]string `json:"expected_headers,omitempty" yaml:"expected_headers,omitempty"`
	ExpectedBody    any               `json:"expected_body,omitempty" yaml:"expected_body,omitempty"`
	ExpectedText    string            `json:"expected_text,omitempty" yaml:"expected_text,omitempty"`
}

// FileConfig defines file upload configuration
type FileConfig struct {
	FieldName   string `json:"field_name" yaml:"field_name"`
	Filename    string `json:"filename" yaml:"filename"`
	Content     string `json:"content" yaml:"content"`
	ContentType string `json:"content_type,omitempty" yaml:"content_type,omitempty"`
}

// ContainerConfig defines resource limits for containers
type ContainerConfig struct {
	CPULimit    string `json:"cpu_limit,omitempty" yaml:"cpu_limit,omitempty"`
	MemoryLimit string `json:"memory_limit,omitempty" yaml:"memory_limit,omitempty"`
}

// ServerConfig defines a server to benchmark
type ServerConfig struct {
	ImageName string           `json:"image_name" yaml:"image_name"`
	Port      int              `json:"port" yaml:"port"`
	Container ContainerConfig  `json:"container,omitempty" yaml:"container,omitempty"`
	Overrides *ServerOverrides `json:"overrides,omitempty" yaml:"overrides,omitempty"`
}

// ServerOverrides allows server-specific parameter overrides
type ServerOverrides struct {
	Workers    *int `json:"workers,omitempty" yaml:"workers,omitempty"`
	Iterations *int `json:"iterations,omitempty" yaml:"iterations,omitempty"`
}

// ResolvedTestCase is a fully merged test case ready for execution
type ResolvedTestCase struct {
	Name            string
	Path            string
	Method          string
	Headers         map[string]string
	Query           map[string]string
	Body            any
	FormData        map[string]string
	File            *FileConfig
	ExpectedStatus  int
	ExpectedHeaders map[string]string
	ExpectedBody    any
	ExpectedText    string
}

// ResolvedConfig contains the resolved global config for a specific server
type ResolvedConfig struct {
	BaseURL    string
	Timeout    time.Duration
	Workers    int
	Iterations int
}
