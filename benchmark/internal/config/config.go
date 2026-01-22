package config

import (
	"fmt"
	"time"
)

type Config struct {
	Global        GlobalConfig              `json:"global"`
	Endpoints     map[string]EndpointConfig `json:"endpoints"`
	Servers       map[string]int            `json:"servers"`
	EndpointOrder []string                  `json:"-"`
}

type GlobalConfig struct {
	BaseURL             string `json:"base_url"`
	Workers             int    `json:"workers"`
	RequestsPerEndpoint int    `json:"requests_per_endpoint"`
	Timeout             string `json:"timeout"`
	CPULimit            string `json:"cpu_limit,omitempty"`
	MemoryLimit         string `json:"memory_limit,omitempty"`
}

func (s *Config) String() string {
	return fmt.Sprintf(
		"=== Configuration ===\nBase URL: %s\nServers: %d | Endpoints: %d\nWorkers: %d | Requests/Endpoint: %d | Timeout: %s\nCPU Limit: %s | Memory Limit: %s\n=======================",
		s.Global.BaseURL,
		len(s.Servers),
		len(s.Endpoints),
		s.Global.Workers,
		s.Global.RequestsPerEndpoint,
		s.Global.Timeout,
		s.Global.CPULimit,
		s.Global.MemoryLimit,
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
	CachedMultipartBody []byte
	CachedContentType   string
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
}
