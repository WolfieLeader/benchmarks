package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"benchmark-client/internal/printer"
)

type Config struct {
	Global        GlobalConfig              `json:"global"`
	Endpoints     map[string]EndpointConfig `json:"endpoints"`
	Servers       map[string]int            `json:"servers"`
	EndpointOrder []string                  `json:"-"`
	ServerOrder   []string                  `json:"-"`
}

type GlobalConfig struct {
	BaseURL             string          `json:"base_url"`
	Workers             int             `json:"workers"`
	RequestsPerEndpoint int             `json:"requests_per_endpoint"`
	Timeout             string          `json:"timeout"`
	CPULimit            string          `json:"cpu_limit,omitempty"`
	MemoryLimit         string          `json:"memory_limit,omitempty"`
	Warmup              WarmupConfig    `json:"warmup,omitzero"`
	Resources           ResourcesConfig `json:"resources,omitzero"`
	Cooldown            string          `json:"cooldown,omitempty"`
	Capacity            CapacityConfig  `json:"capacity,omitzero"`
}

type WarmupConfig struct {
	Enabled             bool `json:"enabled"`
	RequestsPerTestcase int  `json:"requests_per_testcase"`
}

type ResourcesConfig struct {
	Enabled bool `json:"enabled"`
}

type CapacityConfig struct {
	Enabled      bool   `json:"enabled"`
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

func (s *Config) Print() {
	printer.Section("Configuration")

	printer.KeyValue("Base URL", s.Global.BaseURL)
	printer.KeyValuePairs(
		"Servers", strconv.Itoa(len(s.Servers)),
		"Endpoints", strconv.Itoa(len(s.Endpoints)),
	)
	printer.KeyValuePairs(
		"Workers", strconv.Itoa(s.Global.Workers),
		"Requests/Endpoint", strconv.Itoa(s.Global.RequestsPerEndpoint),
		"Timeout", s.Global.Timeout,
	)
	printer.KeyValuePairs(
		"CPU Limit", s.Global.CPULimit,
		"Memory Limit", s.Global.MemoryLimit,
	)

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
	printer.KeyValuePairs("Warmup", warmupStr, "Resources", resourcesStr, "Cooldown", cooldownStr)

	if s.Global.Capacity.Enabled {
		capacityStr := fmt.Sprintf("workers %d-%d, %s measure, precision %s",
			s.Global.Capacity.MinWorkers, s.Global.Capacity.MaxWorkers,
			s.Global.Capacity.Measure, s.Global.Capacity.Precision)
		printer.KeyValue("Capacity", capacityStr)
	} else {
		printer.KeyValue("Capacity", "disabled")
	}
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
	Capacity            CapacityConfig
}
