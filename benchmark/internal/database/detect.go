package database

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// detectExistingStack finds a running repo-owned databases stack, mirroring
// scripts/lib.mts detectDbStack: other compose projects on this host may also
// run a postgres service, so "first postgres container" is not safe — match on
// the com.docker.compose.project.config_files label pointing at our
// infra/docker/databases.yml, and fail loud on an ambiguous match. Zero
// matches is not an error: the caller creates its own stack.
func (m *ComposeManager) detectExistingStack(ctx context.Context) (*Stack, error) {
	out, err := exec.CommandContext(ctx, "docker",
		"ps", "--filter", "label=com.docker.compose.service=postgres", "--format", "{{.ID}}",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("docker ps failed while detecting the DB stack: %w", err)
	}

	ids := strings.Fields(strings.TrimSpace(string(out)))
	if len(ids) == 0 {
		return nil, nil
	}

	isOurs := ourComposeFileMatcher(ctx, m.repoRoot)

	type candidate struct {
		name     string
		project  string
		networks string
	}
	var ours []candidate
	for _, id := range ids {
		inspect, inspectErr := exec.CommandContext(ctx, "docker", //nolint:gosec // ids come from docker ps output
			"inspect", id, "--format",
			`{{.Name}}	{{index .Config.Labels "com.docker.compose.project"}}	{{index .Config.Labels "com.docker.compose.project.config_files"}}	{{range $k,$v := .NetworkSettings.Networks}}{{$k}} {{end}}`,
		).Output()
		if inspectErr != nil {
			return nil, fmt.Errorf("docker inspect %s failed: %w", id, inspectErr)
		}
		// Trim only the trailing newline: TrimSpace would eat the tab before an
		// empty networks field and hide it from the explicit error below.
		parts := strings.SplitN(strings.TrimRight(string(inspect), "\n"), "\t", 4)
		if len(parts) < 4 {
			continue
		}
		name, project, configFiles, networks := parts[0], parts[1], parts[2], parts[3]

		// config_files is a comma-separated list; ours is a single file.
		if slices.ContainsFunc(strings.Split(configFiles, ","), isOurs) {
			ours = append(ours, candidate{name: strings.TrimPrefix(name, "/"), project: project, networks: networks})
		}
	}

	if len(ours) == 0 {
		return nil, nil
	}
	if len(ours) > 1 {
		var list []string
		for _, c := range ours {
			list = append(list, fmt.Sprintf("%s (project=%s, networks=%s)", c.name, c.project, strings.TrimSpace(c.networks)))
		}
		return nil, fmt.Errorf("multiple postgres containers match this repo's databases stack — ambiguous, stop the extras: %s",
			strings.Join(list, "; "))
	}

	stack := ours[0]
	network, err := pickNetwork(stack.project, stack.networks)
	if err != nil {
		return nil, fmt.Errorf("container %s: %w", stack.name, err)
	}
	if network == "" {
		return nil, fmt.Errorf("could not detect the docker network for container %s", stack.name)
	}
	return &Stack{Project: stack.project, Network: network, Owned: false}, nil
}

// pickNetwork chooses the compose network from a container's space-separated
// network list. A container attached to extra networks (docker network
// connect) still belongs to its project's default network; anything else is
// ambiguous and must fail loud rather than yield a mangled network name.
func pickNetwork(project, field string) (string, error) {
	networks := strings.Fields(field)
	switch len(networks) {
	case 0:
		return "", nil
	case 1:
		return networks[0], nil
	}
	def := project + "_default"
	if slices.Contains(networks, def) {
		return def, nil
	}
	return "", fmt.Errorf("attached to multiple networks (%s) and none is %s", strings.Join(networks, ", "), def)
}

// ourComposeFileMatcher reports whether a compose config_files path is this
// repo's databases.yml. Case-insensitive (macOS paths), and anchored to the
// MAIN repo root via git-common-dir so a stack started from any checkout of
// this repo (main clone or a .claude/worktrees agent worktree, which live
// under it) is recognized as ours.
func ourComposeFileMatcher(ctx context.Context, repoRoot string) func(string) bool {
	// The trailing separator is load-bearing: without it a sibling checkout
	// like <root>-backup would match too.
	root := strings.ToLower(mainRepoRoot(ctx, repoRoot)) + string(filepath.Separator)
	suffix := strings.ToLower(string(filepath.Separator) + filepath.Join("infra", "docker", "databases.yml"))
	return func(path string) bool {
		p := strings.ToLower(filepath.Clean(strings.TrimSpace(path)))
		return strings.HasPrefix(p, root) && strings.HasSuffix(p, suffix)
	}
}

func mainRepoRoot(ctx context.Context, repoRoot string) string {
	out, err := exec.CommandContext(ctx, "git", "-C", repoRoot, "rev-parse", "--path-format=absolute", "--git-common-dir").Output() //nolint:gosec // repoRoot is an internal constant path
	if err != nil {
		abs, absErr := filepath.Abs(repoRoot)
		if absErr != nil {
			return repoRoot
		}
		return abs
	}
	return filepath.Dir(strings.TrimSpace(string(out)))
}
