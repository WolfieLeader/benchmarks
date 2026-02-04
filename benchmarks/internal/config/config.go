package config

import (
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
	Url                 string
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

func (cfg *Config) Print() {
	cli.Section("Configuration")

	const disabledStr = "disabled"

	cli.KeyValue("Base URL", cfg.Benchmark.BaseUrl)
	cli.KeyValuePairs(
		"Servers", strconv.Itoa(len(cfg.Servers)),
		"Endpoints", strconv.Itoa(len(cfg.Endpoints)),
	)
	cli.KeyValuePairs(
		"Concurrency", strconv.Itoa(cfg.Benchmark.Concurrency),
		"Duration/Endpoint", cfg.Benchmark.DurationPerEndpoint.String(),
		"Request Timeout", cfg.Benchmark.RequestTimeout.String(),
	)
	cli.KeyValuePairs(
		"CPU Limit", strconv.FormatFloat(cfg.Container.CpuLimit, 'f', -1, 64),
		"Memory Limit", cfg.Container.MemoryLimit,
	)

	cooldownStr := disabledStr
	if strings.TrimSpace(cfg.Benchmark.ServerCooldownRaw) != "" {
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
