package database

import (
	"context"
	"encoding/json/v2"
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

	composeCmd  = "compose"
	projectFlag = "-p"
)

// Stack is the database compose stack a run operates against: either one this
// run created (Owned) or a pre-existing healthy stack it adopted (e.g. from
// `just db-up`). A run must never tear down a stack it didn't create.
type Stack struct {
	Project string
	Network string
	Owned   bool
}

type ComposeManager struct {
	repoRoot      string
	databasesPath string
	grafanaPath   string
	stack         *Stack
}

func NewComposeManager(repoRoot string) *ComposeManager {
	return &ComposeManager{
		repoRoot:      repoRoot,
		databasesPath: filepath.Join(repoRoot, "infra", "docker", "databases.yml"),
		grafanaPath:   filepath.Join(repoRoot, "infra", "docker", "grafana.yml"),
	}
}

func (m *ComposeManager) NetworkName() string {
	if m.stack != nil {
		return m.stack.Network
	}
	return DatabaseProject + "_default"
}

func (m *ComposeManager) Stack() *Stack {
	return m.stack
}

// EnsureDatabases adopts a pre-existing healthy repo-owned DB stack when one is
// running (same label-based detection as scripts/lib.mts), otherwise starts a
// fresh stack under the DatabaseProject name. The returned Stack records
// ownership: only an Owned stack is torn down by StopDatabases.
func (m *ComposeManager) EnsureDatabases(ctx context.Context) (*Stack, error) {
	existing, err := m.detectExistingStack(ctx)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		m.stack = existing
		return existing, nil
	}

	if err := m.composeUp(ctx, m.databasesPath, DatabaseProject); err != nil {
		return nil, err
	}
	m.stack = &Stack{Project: DatabaseProject, Network: DatabaseProject + "_default", Owned: true}
	return m.stack, nil
}

func (m *ComposeManager) StartGrafana(ctx context.Context) error {
	_ = m.composeDown(ctx, m.grafanaPath, GrafanaProject)
	return m.composeUp(ctx, m.grafanaPath, GrafanaProject)
}

// StopDatabases tears down the DB stack only if this run created it; an
// adopted stack (e.g. `just db-up`) is left running untouched — `down -v` on
// a stack the run doesn't own would destroy someone else's volumes.
func (m *ComposeManager) StopDatabases(ctx context.Context) error {
	if m.stack == nil || !m.stack.Owned {
		return nil
	}
	return m.composeDown(ctx, m.databasesPath, m.stack.Project)
}

func (m *ComposeManager) StopGrafana(ctx context.Context) error {
	return m.composeDown(ctx, m.grafanaPath, GrafanaProject)
}

func (m *ComposeManager) composeUp(ctx context.Context, composePath, project string) error {
	args := []string{
		composeCmd,
		"-f", composePath,
		projectFlag, project,
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
		composeCmd,
		"-f", composePath,
		projectFlag, project,
		"down", "-v",
	}
	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // args are controlled internal values
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose down failed for %s: %w\noutput: %s", project, err, out)
	}
	return nil
}

func (m *ComposeManager) projectName() string {
	if m.stack != nil {
		return m.stack.Project
	}
	return DatabaseProject
}

func (m *ComposeManager) WaitHealthy(ctx context.Context, timeout time.Duration, requiredServices []string) error {
	deadline := time.Now().Add(timeout)

	var lastErr error

	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		healthy, err := m.checkServicesHealth(ctx, requiredServices)
		if err != nil {
			lastErr = err
			if !sleepWithContext(ctx, HealthCheckInterval) {
				return ctx.Err()
			}
			continue
		}

		if healthy {
			return nil
		}

		if !sleepWithContext(ctx, HealthCheckInterval) {
			return ctx.Err()
		}
	}

	if lastErr != nil {
		return fmt.Errorf("services did not become healthy within %s: %w", timeout, lastErr)
	}
	return fmt.Errorf("services did not become healthy within %s", timeout)
}

type composeService struct {
	ID     string `json:"ID"`
	Name   string `json:"Name"`
	State  string `json:"State"`
	Health string `json:"Health"`
}

func (m *ComposeManager) listServices(ctx context.Context) ([]composeService, error) {
	args := []string{
		composeCmd,
		projectFlag, m.projectName(),
		"ps", "--format", "json",
	}

	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // args are controlled internal values
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker compose ps failed: %w\noutput: %s", err, out)
	}

	services, err := parseComposeServices(out)
	if err != nil {
		return nil, fmt.Errorf("failed to parse compose ps output: %w", err)
	}
	return services, nil
}

func (m *ComposeManager) checkServicesHealth(ctx context.Context, requiredServices []string) (bool, error) {
	services, err := m.listServices(ctx)
	if err != nil {
		return false, err
	}

	serviceHealth := make(map[string]string)
	for _, svc := range services {
		name := extractServiceName(svc.Name, m.projectName())
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

// DatabaseContainers maps each required database service to its running
// container ID (for per-DB resource sampling during server runs).
func (m *ComposeManager) DatabaseContainers(ctx context.Context, databases []string) (map[string]string, error) {
	services, err := m.listServices(ctx)
	if err != nil {
		return nil, err
	}

	byService := make(map[string]string)
	for _, svc := range services {
		byService[extractServiceName(svc.Name, m.projectName())] = svc.ID
	}

	containers := make(map[string]string, len(databases))
	for _, db := range databases {
		id, ok := byService[db]
		if !ok || id == "" {
			return nil, fmt.Errorf("no running container found for database service %s in project %s", db, m.projectName())
		}
		containers[db] = id
	}
	return containers, nil
}

func parseComposeServices(data []byte) ([]composeService, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, nil
	}

	if strings.HasPrefix(trimmed, "[") {
		var services []composeService
		if err := json.Unmarshal([]byte(trimmed), &services); err != nil {
			return nil, fmt.Errorf("failed to unmarshal services array: %w", err)
		}
		return services, nil
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

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func extractServiceName(containerName, projectName string) string {
	prefix := projectName + "-"
	if name, found := strings.CutPrefix(containerName, prefix); found {
		if index := strings.LastIndex(name, "-"); index > 0 {
			return name[:index]
		}
		return name
	}
	return containerName
}
