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
}

func StartWithOptions(ctx context.Context, timeout time.Duration, opts StartOptions) (Id, error) {
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

	args = append(args, opts.Image)

	cmd := exec.CommandContext(startCtx, "docker", args...)
	out, err := cmd.CombinedOutput()
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
	out, err := cmd.CombinedOutput()
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
		if ctx.Err() != nil {
			return ctx.Err()
		}

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
