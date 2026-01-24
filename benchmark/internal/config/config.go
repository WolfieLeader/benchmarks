package config

import (
	"fmt"
	"strings"
	"time"
)

type Config struct {
	Global        GlobalConfig              `json:"global"`
	Endpoints     map[string]EndpointConfig `json:"endpoints"`
	Servers       map[string]int            `json:"servers"`
	EndpointOrder []string                  `json:"-"`
}

type GlobalConfig struct {
	BaseURL             string          `json:"base_url"`
	Workers             int             `json:"workers"`
	RequestsPerEndpoint int             `json:"requests_per_endpoint"`
	Timeout             string          `json:"timeout"`
	CPULimit            string          `json:"cpu_limit,omitempty"`
	MemoryLimit         string          `json:"memory_limit,omitempty"`
	Warmup              WarmupConfig    `json:"warmup,omitempty"`
	Resources           ResourcesConfig `json:"resources,omitempty"`
	Cooldown            string          `json:"cooldown,omitempty"`
}

type WarmupConfig struct {
	Enabled             bool `json:"enabled"`
	RequestsPerTestcase int  `json:"requests_per_testcase"`
}

type ResourcesConfig struct {
	Enabled          bool `json:"enabled"`
	SampleIntervalMs int  `json:"sample_interval_ms"`
}

func (s *Config) String() string {
	warmupStr := "disabled"
	if s.Global.Warmup.Enabled {
		warmupStr = fmt.Sprintf("%d req/testcase", s.Global.Warmup.RequestsPerTestcase)
	}
	resourcesStr := "disabled"
	if s.Global.Resources.Enabled {
		resourcesStr = "enabled"
	}
	cooldownStr := "disabled"
	if strings.TrimSpace(s.Global.Cooldown) != "" {
		cooldownStr = s.Global.Cooldown
	}
	return fmt.Sprintf(
		"=== Configuration ===\nBase URL: %s\nServers: %d | Endpoints: %d\nWorkers: %d | Requests/Endpoint: %d | Timeout: %s\nCPU Limit: %s | Memory Limit: %s\nWarmup: %s | Resources: %s | Cooldown: %s\n=======================",
		s.Global.BaseURL,
		len(s.Servers),
		len(s.Endpoints),
		s.Global.Workers,
		s.Global.RequestsPerEndpoint,
		s.Global.Timeout,
		s.Global.CPULimit,
		s.Global.MemoryLimit,
		warmupStr,
		resourcesStr,
		cooldownStr,
	)
}

type EndpointConfig struct {
	Method          string            `json:"method"`
	Path            string            `json:"path,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	Query           map[string]string `json:"query,omitempty"`
	Body            any               `json:"body,omitempty"`
	FormData        map[string]string `json:"form_data,omitempty"`
	File            string            `json:"file,omitempty"`
	ExpectedStatus  int               `json:"expected_status,omitempty"`
	ExpectedHeaders map[string]string `json:"expected_headers,omitempty"`
	ExpectedBody    any               `json:"expected_body,omitempty"`
	ExpectedText    string            `json:"expected_text,omitempty"`
	TestCases       []TestCaseConfig  `json:"test_cases,omitempty"`
}

type TestCaseConfig struct {
	Name            string            `json:"name"`
	Path            string            `json:"path,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	Query           map[string]string `json:"query,omitempty"`
	Body            any               `json:"body,omitempty"`
	FormData        map[string]string `json:"form_data,omitempty"`
	File            string            `json:"file,omitempty"`
	ExpectedStatus  int               `json:"expected_status,omitempty"`
	ExpectedHeaders map[string]string `json:"expected_headers,omitempty"`
	ExpectedBody    any               `json:"expected_body,omitempty"`
	ExpectedText    string            `json:"expected_text,omitempty"`
}

type RequestType int

const (
	RequestTypeNone RequestType = iota
	RequestTypeJSON
	RequestTypeForm
	RequestTypeMultipart
)

type FileUpload struct {
	FieldName   string
	Filename    string
	Content     []byte
	ContentType string
}

type Testcase struct {
	EndpointName        string
	Name                string
	Path                string
	URL                 string
	Method              string
	Headers             map[string]string
	RequestType         RequestType
	Body                string
	FormData            map[string]string
	MultipartFields     map[string]string
	FileUpload          *FileUpload
	CachedContentType   string
	CachedFormBody      string
	CachedMultipartBody string
	ExpectedStatus      int
	ExpectedHeaders     map[string]string
	ExpectedBody        any
	ExpectedText        string
}

type ResolvedServer struct {
	Name                string
	ImageName           string
	Port                int
	BaseURL             string
	Timeout             time.Duration
	CPULimit            string
	MemoryLimit         string
	Workers             int
	RequestsPerEndpoint int
	Testcases           []*Testcase
	EndpointOrder       []string
	Warmup              WarmupConfig
	Resources           ResourcesConfig
}
