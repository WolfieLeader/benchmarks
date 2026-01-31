package container

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

const (
	HealthCheckInterval       = 200 * time.Millisecond
	HealthCheckRequestTimeout = 2 * time.Second
)

type Id string

type StartOptions struct {
	Image       string
	Port        int
	HostPort    int
	CPULimit    string
	MemoryLimit string
	Network     string
}

func StartWithOptions(ctx context.Context, timeout time.Duration, opts *StartOptions) (Id, error) {
	startCtx, startCancel := context.WithTimeout(ctx, timeout)
	defer startCancel()

	args := []string{"run", "-d", "--rm"}

	hostPort := opts.HostPort
	if hostPort == 0 {
		hostPort = 8080
	}
	containerPort := opts.Port
	if containerPort == 0 {
		containerPort = 8080
	}
	args = append(args, "-p", fmt.Sprintf("%d:%d", hostPort, containerPort))

	if opts.CPULimit != "" {
		args = append(args, "--cpus="+opts.CPULimit)
	}
	if opts.MemoryLimit != "" {
		args = append(args, "--memory="+opts.MemoryLimit)
	}
	if opts.Network != "" {
		args = append(args, "--network="+opts.Network)
	}

	args = append(args, opts.Image)

	cmd := exec.CommandContext(startCtx, "docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("- Docker run %s failed: %w,\noutput: %s", opts.Image, err, out)
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

	cmd := exec.CommandContext(stopCtx, "docker", "stop", string(containerId)) //nolint:gosec // containerId is controlled internal value
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker stop %s failed: %w,\noutput: %s", containerId, err, out)
	}
	return nil
}

type healthResponse struct {
	Status    string            `json:"status"`
	Databases map[string]string `json:"databases"`
}

func WaitToBeReady(ctx context.Context, timeout time.Duration, serverUrl string) error {
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	deadline := time.Now().Add(timeout)
	url := strings.TrimRight(serverUrl, "/") + "/health"

	var lastErr error

	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		reqCtx, cancel := context.WithTimeout(ctx, HealthCheckRequestTimeout)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, http.NoBody)
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

		if resp.StatusCode != http.StatusOK {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
			time.Sleep(HealthCheckInterval)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("failed to read health response body: %w", err)
			time.Sleep(HealthCheckInterval)
			continue
		}

		var health healthResponse
		if err := json.Unmarshal(body, &health); err != nil {
			lastErr = fmt.Errorf("failed to parse health response: %w", err)
			time.Sleep(HealthCheckInterval)
			continue
		}

		if health.Status != "healthy" {
			lastErr = fmt.Errorf("server status is not healthy: %s", health.Status)
			time.Sleep(HealthCheckInterval)
			continue
		}

		allDbsHealthy := true
		for db, status := range health.Databases {
			if status != "healthy" {
				allDbsHealthy = false
				lastErr = fmt.Errorf("database %s is not healthy: %s", db, status)
				break
			}
		}
		if !allDbsHealthy {
			time.Sleep(HealthCheckInterval)
			continue
		}

		return nil
	}

	if lastErr != nil {
		return fmt.Errorf("server did not become ready in %s: last error: %w", timeout, lastErr)
	}
	return fmt.Errorf("server did not become ready in %s", timeout)
}
