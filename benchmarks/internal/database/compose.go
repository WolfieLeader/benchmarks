package database

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	DefaultProjectName    = "benchmark-dbs"
	HealthCheckInterval   = 2 * time.Second
	DefaultHealthyTimeout = 120 * time.Second
)

type ComposeManager struct {
	composePath string
	projectName string
}

func NewComposeManager(composePath string) *ComposeManager {
	return &ComposeManager{
		composePath: composePath,
		projectName: DefaultProjectName,
	}
}

func (m *ComposeManager) NetworkName() string {
	return m.projectName + "_default"
}

func (m *ComposeManager) Start(ctx context.Context) error {
	args := []string{
		"compose",
		"-f", m.composePath,
		"-p", m.projectName,
		"up", "-d",
	}

	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // args are controlled internal values
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose up failed: %w\noutput: %s", err, out)
	}

	return nil
}

func (m *ComposeManager) Stop(ctx context.Context) error {
	args := []string{
		"compose",
		"-f", m.composePath,
		"-p", m.projectName,
		"down",
	}

	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // args are controlled internal values
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose down failed: %w\noutput: %s", err, out)
	}

	return nil
}

type composeService struct {
	Name   string `json:"Name"`
	State  string `json:"State"`
	Health string `json:"Health"`
}

func (m *ComposeManager) WaitHealthy(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	requiredServices := []string{"postgres", "mongodb", "redis", "cassandra"}

	var lastErr error

	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		healthy, err := m.checkServicesHealth(ctx, requiredServices)
		if err != nil {
			lastErr = err
			time.Sleep(HealthCheckInterval)
			continue
		}

		if healthy {
			return nil
		}

		time.Sleep(HealthCheckInterval)
	}

	if lastErr != nil {
		return fmt.Errorf("services did not become healthy within %s: %w", timeout, lastErr)
	}
	return fmt.Errorf("services did not become healthy within %s", timeout)
}

func (m *ComposeManager) checkServicesHealth(ctx context.Context, requiredServices []string) (bool, error) {
	args := []string{
		"compose",
		"-f", m.composePath,
		"-p", m.projectName,
		"ps", "--format", "json",
	}

	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // args are controlled internal values
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("docker compose ps failed: %w\noutput: %s", err, out)
	}

	services, err := parseComposeServices(out)
	if err != nil {
		return false, fmt.Errorf("failed to parse compose ps output: %w", err)
	}

	serviceHealth := make(map[string]string)
	for _, svc := range services {
		name := extractServiceName(svc.Name, m.projectName)
		serviceHealth[name] = svc.Health
	}

	for _, required := range requiredServices {
		health, exists := serviceHealth[required]
		if !exists {
			return false, fmt.Errorf("service %s not found in compose stack", required)
		}
		if health != "healthy" {
			return false, nil
		}
	}

	return true, nil
}

func parseComposeServices(data []byte) ([]composeService, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, nil
	}

	// docker compose ps --format json outputs one JSON object per line
	lines := strings.Split(trimmed, "\n")
	var services []composeService

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var svc composeService
		if err := json.Unmarshal([]byte(line), &svc); err != nil {
			return nil, fmt.Errorf("failed to unmarshal service JSON: %w", err)
		}
		services = append(services, svc)
	}

	return services, nil
}

func extractServiceName(containerName, projectName string) string {
	// Container names follow pattern: projectName-serviceName-1
	prefix := projectName + "-"
	if name, found := strings.CutPrefix(containerName, prefix); found {
		// Remove the replica suffix (-1, -2, etc.)
		if idx := strings.LastIndex(name, "-"); idx > 0 {
			return name[:idx]
		}
		return name
	}
	return containerName
}
