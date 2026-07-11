package container

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// StartOptions describes one server-under-test container.
type StartOptions struct {
	Image string
	// ContainerPort is the port the server listens on INSIDE the container: the
	// canonical 8080 (PLAN §6 rule 1), sourced from the manifest `port`.
	// testcontainers maps it to a dynamic host port so servers never collide.
	ContainerPort  int
	CpuLimit       float64
	MemoryLimit    string // normalized memory string ("2gb", "512mb", or bare bytes)
	Network        string // docker network to join for DB service-name DNS
	Databases      []string
	StartupTimeout time.Duration
}

// Server is a running server-under-test container with its dynamically mapped
// host endpoint.
type Server struct {
	ctr      testcontainers.Container
	ID       string // full container id (for resource sampling)
	HostPort int    // dynamically mapped host port
	BaseURL  string // e.g. "http://localhost:54123" (no trailing slash)
}

// Start launches the server image via testcontainers-go, applying CPU/memory
// limits, joining the DB network, and waiting until /health and every database's
// /db/<db>/health return 200 before returning. It maps the container port to a
// dynamic host port. On failure it terminates any partially-created container so
// nothing leaks.
func Start(ctx context.Context, opts *StartOptions) (*Server, error) {
	portSpec := fmt.Sprintf("%d/tcp", opts.ContainerPort)

	memBytes, err := memoryLimitBytes(opts.MemoryLimit)
	if err != nil {
		return nil, err
	}

	// Readiness: server first, then each DB dependency it exposes. Every
	// strategy targets the single exposed port (default status matcher = 200).
	strategies := make([]wait.Strategy, 0, len(opts.Databases)+1)
	strategies = append(strategies, wait.ForHTTP("/health"))
	for _, db := range opts.Databases {
		strategies = append(strategies, wait.ForHTTP("/db/"+db+"/health"))
	}
	startupTimeout := opts.StartupTimeout
	if startupTimeout <= 0 {
		startupTimeout = 60 * time.Second
	}

	req := testcontainers.ContainerRequest{
		Image:        opts.Image,
		ExposedPorts: []string{portSpec},
		WaitingFor:   wait.ForAll(strategies...).WithStartupTimeoutDefault(startupTimeout),
		HostConfigModifier: func(hc *container.HostConfig) {
			if opts.CpuLimit > 0 {
				hc.NanoCPUs = int64(opts.CpuLimit * 1e9)
			}
			if memBytes > 0 {
				hc.Memory = memBytes
			}
		},
	}
	if opts.Network != "" {
		req.Networks = []string{opts.Network}
	}

	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		// A failed wait still yields a created container; terminate it so the run
		// doesn't leak one (ryuk would eventually reap it, but not before it
		// contends for resources during this run).
		if ctr != nil {
			_ = ctr.Terminate(context.WithoutCancel(ctx))
		}
		return nil, err
	}

	mapped, err := ctr.MappedPort(ctx, portSpec)
	if err != nil {
		_ = ctr.Terminate(context.WithoutCancel(ctx))
		return nil, fmt.Errorf("map port %s: %w", portSpec, err)
	}
	host, err := ctr.Host(ctx)
	if err != nil {
		_ = ctr.Terminate(context.WithoutCancel(ctx))
		return nil, fmt.Errorf("resolve container host: %w", err)
	}

	hostPort := int(mapped.Num())
	return &Server{
		ctr:      ctr,
		ID:       ctr.GetContainerID(),
		HostPort: hostPort,
		BaseURL:  "http://" + net.JoinHostPort(host, strconv.Itoa(hostPort)),
	}, nil
}

// Stop terminates the container (stop + remove). Ryuk is the backstop; this is
// the explicit, prompt teardown between servers.
func (s *Server) Stop(ctx context.Context) error {
	if s == nil || s.ctr == nil {
		return nil
	}
	return s.ctr.Terminate(ctx)
}

// memoryLimitBytes converts a normalized memory limit (see
// config.normalizeMemoryLimit: a bare number, or number+"kb"/"mb"/"gb") into
// bytes for the docker HostConfig. An empty string means "no limit" (0). Units
// are 1024-based to match the previous `docker run --memory` behavior.
func memoryLimitBytes(limit string) (int64, error) {
	limit = strings.TrimSpace(strings.ToLower(limit))
	if limit == "" {
		return 0, nil
	}

	var mult int64 = 1
	switch {
	case strings.HasSuffix(limit, "gb"):
		mult, limit = 1<<30, strings.TrimSuffix(limit, "gb")
	case strings.HasSuffix(limit, "mb"):
		mult, limit = 1<<20, strings.TrimSuffix(limit, "mb")
	case strings.HasSuffix(limit, "kb"):
		mult, limit = 1<<10, strings.TrimSuffix(limit, "kb")
	}

	value, err := strconv.ParseFloat(strings.TrimSpace(limit), 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid memory limit %q", limit)
	}
	return int64(value * float64(mult)), nil
}
