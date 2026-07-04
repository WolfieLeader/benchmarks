package database

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOurComposeFileMatcher(t *testing.T) {
	t.Parallel()

	// The matcher anchors on the resolved main repo root of THIS checkout, so
	// build expectations from it rather than hardcoding a path.
	root := mainRepoRoot(context.Background(), ".")
	isOurs := ourComposeFileMatcher(context.Background(), ".")

	cases := []struct {
		name string
		path string
		want bool
	}{
		{"exact", filepath.Join(root, "infra", "docker", "databases.yml"), true},
		{"case-insensitive (macOS)", filepath.Join(root, "Infra", "Docker", "DATABASES.yml"), true},
		{"agent worktree under the main root", filepath.Join(root, ".claude", "worktrees", "agent-x", "infra", "docker", "databases.yml"), true},
		{"other repo's databases.yml", "/somewhere/else/infra/docker/databases.yml", false},
		{"sibling checkout sharing the root as a name prefix", filepath.Join(root+"-backup", "infra", "docker", "databases.yml"), false},
		{"our repo but a different compose file", filepath.Join(root, "infra", "docker", "grafana.yml"), false},
		{"surrounding whitespace trimmed", "  " + filepath.Join(root, "infra", "docker", "databases.yml") + "  ", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isOurs(tc.path); got != tc.want {
				t.Errorf("isOurs(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

// A run must never tear down a stack it didn't create: StopDatabases is a
// no-op for an adopted stack and before any stack exists.
func TestStopDatabasesRespectsOwnership(t *testing.T) {
	t.Parallel()

	m := NewComposeManager("..")
	if err := m.StopDatabases(context.Background()); err != nil {
		t.Errorf("StopDatabases with no stack: %v", err)
	}

	m.stack = &Stack{Project: "docker", Network: "docker_default", Owned: false}
	if err := m.StopDatabases(context.Background()); err != nil {
		t.Errorf("StopDatabases on adopted stack must be a no-op, got %v", err)
	}
	if m.NetworkName() != "docker_default" {
		t.Errorf("NetworkName: got %q, want the adopted stack's network", m.NetworkName())
	}
	if m.projectName() != "docker" {
		t.Errorf("projectName: got %q, want the adopted stack's project", m.projectName())
	}
}

func TestPickNetwork(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		field   string
		want    string
		wantErr bool
	}{
		{"no networks", "", "", false},
		{"single network", "docker_default ", "docker_default", false},
		{"extra network but default present", "metrics docker_default ", "docker_default", false},
		{"multiple networks, no default", "metrics other ", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := pickNetwork("docker", tc.field)
			if (err != nil) != tc.wantErr {
				t.Fatalf("pickNetwork(%q) error = %v, wantErr %v", tc.field, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("pickNetwork(%q) = %q, want %q", tc.field, got, tc.want)
			}
		})
	}
}

func TestParseComposeServicesWithIDs(t *testing.T) {
	t.Parallel()

	// docker compose ps --format json emits one object per line.
	out := []byte(`{"ID":"abc123","Name":"docker-postgres-1","State":"running","Health":"healthy"}
{"ID":"def456","Name":"docker-redis-1","State":"running","Health":"healthy"}`)

	services, err := parseComposeServices(out)
	if err != nil {
		t.Fatalf("parseComposeServices: %v", err)
	}
	if len(services) != 2 || services[0].ID != "abc123" || services[1].ID != "def456" {
		t.Errorf("IDs not parsed: %+v", services)
	}
	if extractServiceName(services[0].Name, "docker") != "postgres" {
		t.Errorf("service name extraction broke: %q", extractServiceName(services[0].Name, "docker"))
	}
}
