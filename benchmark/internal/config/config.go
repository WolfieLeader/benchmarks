package config

import (
	"fmt"
	"strconv"
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
	EndpointName string
	Name         string
	Path         string
	// RequestURI is the relative request target (path + encoded query, e.g.
	// "/params/search?limit=10"). The absolute URL is formed at request time as
	// baseURL + RequestURI, so testcases are reusable across servers whose host
	// ports testcontainers maps dynamically.
	RequestURI          string
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
	BaseUrl             string
	RequestTimeout      time.Duration
	CpuLimit            float64
	MemoryLimit         string
	Concurrency         int
	Load                LoadConfig
	DurationPerEndpoint time.Duration
	Testcases           []*Testcase
	EndpointOrder       []string
	WarmupDuration      time.Duration
	WarmupPause         time.Duration
	Sequences           []*ResolvedSequence
}

type RuntimeOptions struct {
	Servers []string // empty means all servers
}

func GetServerNames(servers []*ResolvedServer) []string {
	names := make([]string, len(servers))
	for i, s := range servers {
		names[i] = s.Name
	}
	return names
}

func (cfg *Config) Print(serverCount int) {
	cli.Section("Configuration")

	const disabledStr = "disabled"

	cli.KeyValue("Base URL", cfg.Benchmark.BaseUrl)
	cli.KeyValuePairs(
		"Servers", strconv.Itoa(serverCount),
		"Endpoints", strconv.Itoa(len(cfg.Endpoints)),
	)
	cli.KeyValuePairs(
		"Concurrency", strconv.Itoa(cfg.Benchmark.Concurrency),
		"Duration/Endpoint", cfg.Benchmark.DurationPerEndpoint.String(),
		"Request Timeout", cfg.Benchmark.RequestTimeout.String(),
	)
	if cfg.Benchmark.Load.Mode == LoadModeOpen {
		rateStr := strconv.FormatFloat(cfg.Benchmark.Load.Rate, 'f', -1, 64) + " req/s"
		if len(cfg.Benchmark.Load.Stages) > 0 {
			rateStr += fmt.Sprintf(" + %d stages", len(cfg.Benchmark.Load.Stages))
		}
		cli.KeyValuePairs(
			"Load Mode", cfg.Benchmark.Load.Mode,
			"Arrival Rate", rateStr,
			"Max In-Flight", strconv.Itoa(cfg.Benchmark.Load.MaxInFlight),
		)
	}
	cli.KeyValuePairs(
		"CPU Limit", strconv.FormatFloat(cfg.Container.CpuLimit, 'f', -1, 64),
		"Memory Limit", cfg.Container.MemoryLimit,
	)

	cooldownStr := disabledStr
	if cfg.Benchmark.ServerCooldown > 0 {
		cooldownStr = cfg.Benchmark.ServerCooldown.String()
	}
	cli.KeyValuePairs(
		"Warmup", cfg.Benchmark.WarmupDuration.String(),
		"Warmup Pause", cfg.Benchmark.WarmupPause.String(),
		"Server Cooldown", cooldownStr,
	)
}

func ApplyRuntimeOptions(servers []*ResolvedServer, opts *RuntimeOptions) (filtered []*ResolvedServer, invalidNames []string) {
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
