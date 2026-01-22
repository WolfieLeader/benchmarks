package container

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

const (
	// HealthCheckInterval is the time between health check attempts
	HealthCheckInterval = 200 * time.Millisecond
	// HealthCheckRequestTimeout is the timeout for each health check request
	HealthCheckRequestTimeout = 2 * time.Second
)

type Id string

// StartOptions contains configuration for starting a container
type StartOptions struct {
	Image       string // Docker image name
	Port        int    // Container's internal port (e.g., 3005)
	HostPort    int    // Host port to map to (default: 8080)
	CPULimit    string // CPU limit (e.g., "1.0" for 1 CPU, "0.5" for half)
	MemoryLimit string // Memory limit (e.g., "512m", "1g")
}

// StartWithOptions starts a container with the specified options
// Returns the container ID and the actual host port used
func StartWithOptions(ctx context.Context, timeout time.Duration, opts StartOptions) (Id, error) {
	startCtx, startCancel := context.WithTimeout(ctx, timeout)
	defer startCancel()

	// Build docker run arguments
	args := []string{"run", "-d", "--rm"}

	// Port mapping
	hostPort := opts.HostPort
	if hostPort == 0 {
		hostPort = 8080
	}
	containerPort := opts.Port
	if containerPort == 0 {
		containerPort = 8080
	}
	args = append(args, "-p", fmt.Sprintf("%d:%d", hostPort, containerPort))

	// Resource limits
	if opts.CPULimit != "" {
		args = append(args, "--cpus="+opts.CPULimit)
	}
	if opts.MemoryLimit != "" {
		args = append(args, "--memory="+opts.MemoryLimit)
	}

	// Image name
	args = append(args, opts.Image)

	cmd := exec.CommandContext(startCtx, "docker", args...)
	out, err := cmd.CombinedOutput() // stdout + stderr
	if err != nil {
		return "", fmt.Errorf("- Docker run %s failed: %v,\noutput: %s", opts.Image, err, out)
	}

	id := strings.TrimSpace(string(out))
	if len(id) > 12 {
		id = id[:12]
	}
	return Id(id), nil
}

func Stop(ctx context.Context, timeout time.Duration, containerId Id) error {
	stopCtx, stopCancel := context.WithTimeout(ctx, timeout)
	defer stopCancel()

	cmd := exec.CommandContext(stopCtx, "docker", "stop", string(containerId))
	out, err := cmd.CombinedOutput() // stdout + stderr
	if err != nil {
		return fmt.Errorf("Docker stop %s failed: %v,\noutput: %s", containerId, err, out)
	}
	return nil
}

func WaitToBeReady(ctx context.Context, timeout time.Duration, serverUrl string) error {
	client := http.Client{}
	deadline := time.Now().Add(timeout)
	url := strings.TrimRight(serverUrl, "/") + "/health"

	var lastErr error

	for time.Now().Before(deadline) {
		// Check context cancellation first
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Create request with context timeout
		reqCtx, cancel := context.WithTimeout(ctx, HealthCheckRequestTimeout)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
		if err != nil {
			cancel()
			return fmt.Errorf("failed to create health check request: %w", err)
		}

		resp, err := client.Do(req)
		cancel()

		if err != nil {
			lastErr = err
			time.Sleep(HealthCheckInterval)
			continue
		}

		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
			time.Sleep(HealthCheckInterval)
			continue
		}
		return nil
	}

	if lastErr != nil {
		return fmt.Errorf("server did not become ready in %s: last error: %v", timeout, lastErr)
	}
	return fmt.Errorf("server did not become ready in %s", timeout)
}
