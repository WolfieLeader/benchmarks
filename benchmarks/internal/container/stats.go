package container

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
)

var dockerStatsClient = &http.Client{
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", "/var/run/docker.sock")
		},
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 10,
	},
}

type ResourceStats struct {
	Memory   MemoryStats `json:"memory"`
	CPU      CPUStats    `json:"cpu"`
	Samples  int         `json:"samples"`
	Warnings []string    `json:"warnings,omitempty"`
}

type MemoryStats struct {
	MinBytes float64 `json:"min_bytes"`
	AvgBytes float64 `json:"avg_bytes"`
	MaxBytes float64 `json:"max_bytes"`
}

type CPUStats struct {
	MinPercent float64 `json:"min_percent"`
	AvgPercent float64 `json:"avg_percent"`
	MaxPercent float64 `json:"max_percent"`
}

type ResourceSampler struct {
	containerID string

	mu      sync.Mutex
	memory  []uint64
	cpu     []float64
	running bool
	stopCh  chan struct{}
	doneCh  chan struct{}
}

type cpuStatsBlock struct {
	CPUUsage struct {
		TotalUsage uint64 `json:"total_usage"`
	} `json:"cpu_usage"`
	SystemCPUUsage uint64 `json:"system_cpu_usage"`
	OnlineCPUs     int    `json:"online_cpus"`
}

type dockerStatsAPI struct {
	MemoryStats struct {
		Usage uint64 `json:"usage"`
	} `json:"memory_stats"`
	CPUStats    cpuStatsBlock `json:"cpu_stats"`
	PreCPUStats cpuStatsBlock `json:"precpu_stats"`
}

func NewResourceSampler(containerID string) *ResourceSampler {
	return &ResourceSampler{
		containerID: containerID,
		memory:      make([]uint64, 0, 64),
		cpu:         make([]float64, 0, 64),
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}
}

func (r *ResourceSampler) Start(ctx context.Context) {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	r.running = true
	r.mu.Unlock()

	go r.stream(ctx)
}

func (r *ResourceSampler) Stop() ResourceStats {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return ResourceStats{}
	}
	r.running = false
	r.mu.Unlock()

	close(r.stopCh)
	<-r.doneCh

	return r.aggregate()
}

func (r *ResourceSampler) stream(ctx context.Context) {
	defer close(r.doneCh)

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		select {
		case <-r.stopCh:
			cancel()
		case <-streamCtx.Done():
		}
	}()

	url := fmt.Sprintf("http://localhost/containers/%s/stats?stream=true", r.containerID)
	req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return
	}

	resp, err := dockerStatsClient.Do(req)
	if err != nil {
		return
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			return
		}
	}()

	decoder := json.NewDecoder(resp.Body)
	for {
		var stats dockerStatsAPI
		if err := decoder.Decode(&stats); err != nil {
			return // Connection closed or context canceled
		}

		r.processSample(&stats)
	}
}

func (r *ResourceSampler) processSample(stats *dockerStatsAPI) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.memory = append(r.memory, stats.MemoryStats.Usage)

	currCPU := stats.CPUStats.CPUUsage.TotalUsage
	prevCPU := stats.PreCPUStats.CPUUsage.TotalUsage
	currSys := stats.CPUStats.SystemCPUUsage
	prevSys := stats.PreCPUStats.SystemCPUUsage
	numCPUs := stats.CPUStats.OnlineCPUs
	if numCPUs == 0 {
		numCPUs = 1
	}

	if currSys > prevSys && currCPU >= prevCPU {
		cpuDelta := currCPU - prevCPU
		sysDelta := currSys - prevSys

		cpuPercent := (float64(cpuDelta) / float64(sysDelta)) * float64(numCPUs) * 100.0
		if cpuPercent > float64(numCPUs)*100 {
			cpuPercent = float64(numCPUs) * 100
		}
		r.cpu = append(r.cpu, cpuPercent)
	}
}

func (r *ResourceSampler) aggregate() ResourceStats {
	r.mu.Lock()
	memory := r.memory
	cpu := r.cpu
	r.mu.Unlock()

	result := ResourceStats{}
	var warnings []string

	if len(memory) > 0 {
		minMem, maxMem := memory[0], memory[0]
		var totalMem uint64
		for _, m := range memory {
			if m < minMem {
				minMem = m
			}
			if m > maxMem {
				maxMem = m
			}
			totalMem += m
		}
		result.Memory = MemoryStats{
			MinBytes: float64(minMem),
			AvgBytes: float64(totalMem) / float64(len(memory)),
			MaxBytes: float64(maxMem),
		}
		result.Samples = len(memory)
	}

	if len(cpu) > 0 {
		minCPU, maxCPU := cpu[0], cpu[0]
		var totalCPU float64
		for _, c := range cpu {
			if c < minCPU {
				minCPU = c
			}
			if c > maxCPU {
				maxCPU = c
			}
			totalCPU += c
		}
		result.CPU = CPUStats{
			MinPercent: minCPU,
			AvgPercent: totalCPU / float64(len(cpu)),
			MaxPercent: maxCPU,
		}
	}

	if result.Samples < 3 {
		warnings = append(warnings, "low samples")
	}

	if len(warnings) > 0 {
		result.Warnings = warnings
	}

	return result
}
