package database

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	DatabaseProject       = "benchmark-dbs"
	GrafanaProject        = "benchmark-grafana"
	HealthCheckInterval   = 2 * time.Second
	DefaultHealthyTimeout = 120 * time.Second
)

type ComposeManager struct {
	databasesPath string
	grafanaPath   string
}

func NewComposeManager(repoRoot string) *ComposeManager {
	return &ComposeManager{
		databasesPath: filepath.Join(repoRoot, "infra", "compose", "databases.yml"),
		grafanaPath:   filepath.Join(repoRoot, "infra", "compose", "grafana.yml"),
	}
}

func (m *ComposeManager) NetworkName() string {
	return DatabaseProject + "_default"
}

func (m *ComposeManager) StartDatabases(ctx context.Context) error {
	return m.composeUp(ctx, m.databasesPath, DatabaseProject)
}

func (m *ComposeManager) StartGrafana(ctx context.Context) error {
	// Clean up any existing containers first for a fresh start
	_ = m.composeDown(ctx, m.grafanaPath, GrafanaProject)
	return m.composeUp(ctx, m.grafanaPath, GrafanaProject)
}

func (m *ComposeManager) StopDatabases(ctx context.Context) error {
	return m.composeDown(ctx, m.databasesPath, DatabaseProject)
}

func (m *ComposeManager) StopGrafana(ctx context.Context) error {
	return m.composeDown(ctx, m.grafanaPath, GrafanaProject)
}

func (m *ComposeManager) composeUp(ctx context.Context, composePath, project string) error {
	args := []string{
		"compose",
		"-f", composePath,
		"-p", project,
		"up", "-d",
	}
	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // args are controlled internal values
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose up failed for %s: %w\noutput: %s", project, err, out)
	}
	return nil
}

func (m *ComposeManager) composeDown(ctx context.Context, composePath, project string) error {
	args := []string{
		"compose",
		"-f", composePath,
		"-p", project,
		"down",
	}
	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // args are controlled internal values
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose down failed for %s: %w\noutput: %s", project, err, out)
	}
	return nil
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

type composeService struct {
	Name   string `json:"Name"`
	State  string `json:"State"`
	Health string `json:"Health"`
}

func (m *ComposeManager) checkServicesHealth(ctx context.Context, requiredServices []string) (bool, error) {
	args := []string{
		"compose",
		"-p", DatabaseProject,
		"ps", "--format", "json",
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
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
		name := extractServiceName(svc.Name, DatabaseProject)
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
	prefix := projectName + "-"
	if name, found := strings.CutPrefix(containerName, prefix); found {
		if idx := strings.LastIndex(name, "-"); idx > 0 {
			return name[:idx]
		}
		return name
	}
	return containerName
}
