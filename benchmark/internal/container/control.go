package container

import (
	"context"
	"os/exec"
	"strings"
)

// This file holds the image-existence preflight only. The server lifecycle
// (start / readiness / stop) lives in lifecycle.go and is driven by
// testcontainers-go (PLAN §7.3); resource sampling lives in stats.go.
//
// The preflight stays on the docker CLI on purpose: the benchmark requires
// images to be pre-built (`just images`) and fails fast with that hint before
// testcontainers would otherwise try to pull a local-only `bench/*` image from a
// registry and produce a confusing error.

func ImageExists(ctx context.Context, imageName string) bool {
	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", imageName) //nolint:gosec // imageName is from trusted config
	if cmd.Run() == nil {
		return true
	}
	if hasTagOrDigest(imageName) {
		return false
	}
	cmd = exec.CommandContext(ctx, "docker", "image", "inspect", imageName+":latest") //nolint:gosec // imageName is from trusted config
	return cmd.Run() == nil
}

func hasTagOrDigest(imageName string) bool {
	if strings.Contains(imageName, "@") {
		return true
	}
	lastSlash := strings.LastIndex(imageName, "/")
	lastColon := strings.LastIndex(imageName, ":")
	return lastColon > lastSlash
}

func CheckImages(ctx context.Context, imageNames []string) []string {
	var missing []string
	refs, err := listImageRefs(ctx)
	if err == nil && len(refs) > 0 {
		for _, name := range imageNames {
			if imageRefExists(name, refs) {
				continue
			}
			if ImageExists(ctx, name) {
				continue
			}
			missing = append(missing, name)
		}
		return missing
	}
	for _, name := range imageNames {
		if !ImageExists(ctx, name) {
			missing = append(missing, name)
		}
	}
	return missing
}

func listImageRefs(ctx context.Context) (map[string]struct{}, error) {
	cmd := exec.CommandContext(ctx, "docker", "images", "--format", "{{.Repository}}:{{.Tag}}")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	refs := make(map[string]struct{})
	for line := range strings.SplitSeq(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "<none>") {
			continue
		}
		refs[line] = struct{}{}
	}
	return refs, nil
}

func imageRefExists(imageName string, refs map[string]struct{}) bool {
	if strings.Contains(imageName, "@") {
		return false
	}
	if hasTagOrDigest(imageName) {
		_, ok := refs[imageName]
		return ok
	}
	_, ok := refs[imageName+":latest"]
	return ok
}
