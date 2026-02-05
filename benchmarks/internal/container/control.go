package container

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func ImageExists(ctx context.Context, imageName string) bool {
	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", imageName)
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

const (
	HealthCheckInterval       = 200 * time.Millisecond
	HealthCheckRequestTimeout = 2 * time.Second
)

type Id string

type StartOptions struct {
	Image       string
	Port        int
	HostPort    int
	CpuLimit    float64
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

	if opts.CpuLimit > 0 {
		args = append(args, "--cpus="+strconv.FormatFloat(opts.CpuLimit, 'f', -1, 64))
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

	// Use -t 2 for a 2-second grace period instead of default 10 seconds
	cmd := exec.CommandContext(stopCtx, "docker", "stop", "-t", "2", string(containerId)) //nolint:gosec // containerId is controlled internal value
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker stop %s failed: %w,\noutput: %s", containerId, err, out)
	}
	return nil
}

func checkHealth(ctx context.Context, client *http.Client, url string) error {
	reqCtx, cancel := context.WithTimeout(ctx, HealthCheckRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}

func WaitToBeReady(ctx context.Context, timeout time.Duration, serverUrl string, requiredDbs []string) error {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	deadline := time.Now().Add(timeout)
	baseUrl := strings.TrimRight(serverUrl, "/")

	var lastErr error

	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err := checkHealth(ctx, client, baseUrl+"/health"); err != nil {
			lastErr = fmt.Errorf("server health check failed: %w", err)
			time.Sleep(HealthCheckInterval)
			continue
		}

		allDbsHealthy := true
		for _, db := range requiredDbs {
			if err := checkHealth(ctx, client, baseUrl+"/db/"+db+"/health"); err != nil {
				allDbsHealthy = false
				lastErr = fmt.Errorf("database %s health check failed: %w", db, err)
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
