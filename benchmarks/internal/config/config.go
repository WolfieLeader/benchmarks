package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"benchmark-client/internal/cli"
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
	Name                string
	ImageName           string
	Port                int
	BaseURL             string
	RequestTimeout      time.Duration
	CPULimit            float64
	MemoryLimit         string
	Concurrency         int
	RequestsPerEndpoint int
	Testcases           []*Testcase
	EndpointOrder       []string
	WarmupEnabled       bool
	WarmupDuration      time.Duration
	WarmupPause         time.Duration
	ResourcesEnabled    bool
	Capacity            CapacityConfig
	Flows               []*ResolvedFlow
}

type RuntimeOptions struct {
	Warmup    bool
	Resources bool
	Capacity  bool
	Servers   []string // empty means all servers
}

func GetServerNames(servers []*ResolvedServer) []string {
	names := make([]string, len(servers))
	for i, s := range servers {
		names[i] = s.Name
	}
	return names
}

func (cfg *Config) Print() {
	cli.Section("Configuration")

	const disabledStr = "disabled"

	cli.KeyValue("Base URL", cfg.Benchmark.BaseURL)
	cli.KeyValuePairs(
		"Servers", strconv.Itoa(len(cfg.Servers)),
		"Endpoints", strconv.Itoa(len(cfg.Endpoints)),
	)
	cli.KeyValuePairs(
		"Concurrency", strconv.Itoa(cfg.Benchmark.Concurrency),
		"Requests/Endpoint", strconv.Itoa(cfg.Benchmark.RequestsPerEndpoint),
		"Request Timeout", cfg.Benchmark.RequestTimeout.String(),
	)
	cli.KeyValuePairs(
		"CPU Limit", strconv.FormatFloat(cfg.Container.CPULimit, 'f', -1, 64),
		"Memory Limit", cfg.Container.MemoryLimit,
	)

	warmupStr := disabledStr
	if cfg.Benchmark.WarmupEnabled {
		warmupStr = cfg.Benchmark.WarmupDuration.String()
	}
	warmupPauseStr := disabledStr
	if cfg.Benchmark.WarmupEnabled {
		warmupPauseStr = cfg.Benchmark.WarmupPause.String()
	}
	resourcesStr := disabledStr
	if cfg.Benchmark.ResourcesEnabled {
		resourcesStr = "enabled"
	}
	cooldownStr := disabledStr
	if strings.TrimSpace(cfg.Benchmark.ServerCooldownRaw) != "" {
		cooldownStr = cfg.Benchmark.ServerCooldown.String()
	}
	cli.KeyValuePairs("Warmup", warmupStr, "Warmup Pause", warmupPauseStr, "Resources", resourcesStr, "Server Cooldown", cooldownStr)

	if cfg.Capacity.Enabled {
		capacityStr := fmt.Sprintf("concurrency %d-%d, precision %s",
			cfg.Capacity.MinConcurrency, cfg.Capacity.MaxConcurrency, cfg.Capacity.SearchPrecision)
		cli.KeyValue("Capacity", capacityStr)
		cli.KeyValuePairs(
			"Min Success Rate", cfg.Capacity.MinSuccessRate,
			"P99 Threshold", cfg.Capacity.P99LatencyThreshold,
		)
		cli.KeyValuePairs(
			"Pre-Run Pause", cfg.Capacity.PreRunPause.String(),
			"Warmup", cfg.Capacity.WarmupDuration.String(),
			"Measure", cfg.Capacity.MeasureDuration.String(),
			"Iteration Pause", cfg.Capacity.IterationPause.String(),
		)
	} else {
		cli.KeyValue("Capacity", disabledStr)
	}
}

func ApplyRuntimeOptions(cfg *Config, servers []*ResolvedServer, opts *RuntimeOptions) (filtered []*ResolvedServer, invalidNames []string) {
	cfg.Benchmark.WarmupEnabled = opts.Warmup
	cfg.Benchmark.ResourcesEnabled = opts.Resources
	cfg.Capacity.Enabled = opts.Capacity

	for _, s := range servers {
		s.WarmupEnabled = opts.Warmup
		s.ResourcesEnabled = opts.Resources
		s.Capacity.Enabled = opts.Capacity
	}

	if len(opts.Servers) > 0 {
		available := make(map[string]*ResolvedServer, len(servers))
		for _, s := range servers {
			available[s.Name] = s
		}

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
