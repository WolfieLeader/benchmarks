package config

import (
	"time"
)

type Config struct {
	Global    GlobalConfig              `json:"global"`
	Endpoints map[string]EndpointConfig `json:"endpoints"`
	Servers   map[string]int            `json:"servers"`
}

type GlobalConfig struct {
	BaseURL             string `json:"base_url"`
	Workers             int    `json:"workers"`
	RequestsPerEndpoint int    `json:"requests_per_endpoint"`
	Timeout             string `json:"timeout"`
	CPULimit            string `json:"cpu_limit,omitempty"`
	MemoryLimit         string `json:"memory_limit,omitempty"`
}

type EndpointConfig struct {
	Method          string            `json:"method"`
	Path            string            `json:"path,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	Query           map[string]string `json:"query,omitempty"`
	Body            any               `json:"body,omitempty"`
	FormData        map[string]string `json:"form_data,omitempty"`
	File            *FileConfig       `json:"file,omitempty"`
	ExpectedStatus  int               `json:"expected_status,omitempty"`
	ExpectedHeaders map[string]string `json:"expected_headers,omitempty"`
	ExpectedBody    any               `json:"expected_body,omitempty"`
	ExpectedText    string            `json:"expected_text,omitempty"`
	TestCases       []TestCaseConfig  `json:"test_cases,omitempty"`
}

type FileConfig struct {
	FieldName   string `json:"field_name"`
	Filename    string `json:"filename"`
	Content     string `json:"content"`
	ContentType string `json:"content_type,omitempty"`
}

type TestCaseConfig struct {
	Name            string            `json:"name"`
	Path            string            `json:"path,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	Query           map[string]string `json:"query,omitempty"`
	Body            any               `json:"body,omitempty"`
	FormData        map[string]string `json:"form_data,omitempty"`
	File            *FileConfig       `json:"file,omitempty"`
	ExpectedStatus  int               `json:"expected_status,omitempty"`
	ExpectedHeaders map[string]string `json:"expected_headers,omitempty"`
	ExpectedBody    any               `json:"expected_body,omitempty"`
	ExpectedText    string            `json:"expected_text,omitempty"`
}

type ResolvedTestCase struct {
	EndpointName    string
	Name            string
	Method          string
	Path            string
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

type ResolvedServer struct {
	Name       string
	ImageName  string
	Port       int
	Workers    int
	Iterations int
	TestCases  []*ResolvedTestCase
}

type ResolvedConfig struct {
	BaseURL     string
	Timeout     time.Duration
	Workers     int
	Iterations  int
	CPULimit    string
	MemoryLimit string
	Servers     []*ResolvedServer
}
