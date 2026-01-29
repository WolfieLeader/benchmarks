package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"benchmark-client/internal/printer"
)

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
	Name                      string
	ImageName                 string
	Port                      int
	BaseURL                   string
	Timeout                   time.Duration
	CPULimit                  string
	MemoryLimit               string
	Workers                   int
	RequestsPerEndpoint       int
	Testcases                 []*Testcase
	EndpointOrder             []string
	WarmupRequestsPerTestcase int
	WarmupEnabled             bool
	ResourcesEnabled          bool
	Capacity                  CapacityConfig
	Flows                     []*ResolvedFlow
}

// RuntimeOptions holds the user's selections for benchmark phases (set via CLI)
type RuntimeOptions struct {
	Warmup    bool
	Resources bool
	Capacity  bool
	Servers   []string // empty means all servers
}

// GetServerNames returns a list of server names from resolved servers
func GetServerNames(servers []*ResolvedServer) []string {
	names := make([]string, len(servers))
	for i, s := range servers {
		names[i] = s.Name
	}
	return names
}

func (cfg *ConfigV2) Print() {
	printer.Section("Configuration")

	printer.KeyValue("Base URL", cfg.Benchmark.BaseURL)
	printer.KeyValuePairs(
		"Servers", strconv.Itoa(len(cfg.Servers)),
		"Endpoints", strconv.Itoa(len(cfg.Endpoints)),
	)
	printer.KeyValuePairs(
		"Workers", strconv.Itoa(cfg.Benchmark.Workers),
		"Requests/Endpoint", strconv.Itoa(cfg.Benchmark.Requests),
		"Timeout", cfg.Benchmark.Timeout,
	)
	printer.KeyValuePairs(
		"CPU Limit", cfg.Container.CPU,
		"Memory Limit", cfg.Container.Memory,
	)

	warmupStr := "disabled"
	if cfg.Benchmark.WarmupEnabled {
		warmupStr = fmt.Sprintf("%d req/testcase", cfg.Benchmark.Warmup)
	}
	resourcesStr := "disabled"
	if cfg.Benchmark.ResourcesEnabled {
		resourcesStr = "enabled"
	}
	cooldownStr := "disabled"
	if strings.TrimSpace(cfg.Benchmark.Cooldown) != "" {
		cooldownStr = cfg.Benchmark.Cooldown
	}
	printer.KeyValuePairs("Warmup", warmupStr, "Resources", resourcesStr, "Cooldown", cooldownStr)

	if cfg.Capacity.Enabled {
		capacityStr := fmt.Sprintf("workers %d-%d, precision %s",
			cfg.Capacity.MinWorkers, cfg.Capacity.MaxWorkers, cfg.Capacity.Precision)
		printer.KeyValue("Capacity", capacityStr)
		printer.KeyValuePairs(
			"Success Rate", cfg.Capacity.SuccessRate,
			"P99 Threshold", cfg.Capacity.P99Threshold,
		)
		printer.KeyValuePairs(
			"Warmup", cfg.Capacity.WarmupDuration.String(),
			"Measure", cfg.Capacity.MeasureDuration.String(),
		)
	} else {
		printer.KeyValue("Capacity", "disabled")
	}
}

// ApplyRuntimeOptionsV2 applies CLI options to the v2 config and filters servers.
// Returns the filtered servers and any invalid server names that were requested.
func ApplyRuntimeOptionsV2(cfg *ConfigV2, servers []*ResolvedServer, opts *RuntimeOptions) (filtered []*ResolvedServer, invalidNames []string) {
	// Apply phase flags to config
	cfg.Benchmark.WarmupEnabled = opts.Warmup
	cfg.Benchmark.ResourcesEnabled = opts.Resources
	cfg.Capacity.Enabled = opts.Capacity

	// Apply to all resolved servers
	for _, s := range servers {
		s.WarmupEnabled = opts.Warmup
		s.ResourcesEnabled = opts.Resources
		s.Capacity.Enabled = opts.Capacity
	}

	// Filter servers if specific ones are requested
	if len(opts.Servers) > 0 {
		// Build map of available servers for O(1) lookup
		available := make(map[string]*ResolvedServer, len(servers))
		for _, s := range servers {
			available[s.Name] = s
		}

		// Single pass: collect valid servers and track invalid names
		filtered = make([]*ResolvedServer, 0, len(opts.Servers))
		for _, name := range opts.Servers {
			if s, ok := available[name]; ok {
				filtered = append(filtered, s)
			} else {
				invalidNames = append(invalidNames, name)
			}
		}
		return filtered, invalidNames
	}

	return servers, nil
}
