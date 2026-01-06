package container

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

type Id string

func Start(ctx context.Context, timeout time.Duration, image string) (Id, error) {
	startCtx, startCancel := context.WithTimeout(ctx, timeout)
	defer startCancel()

	cmd := exec.CommandContext(startCtx, "docker", "run", "-d", "--rm", "-p", "8080:8080", image)
	out, err := cmd.CombinedOutput() // stdout + stderr
	if err != nil {
		return "", fmt.Errorf("- Docker run %s failed: %v,\noutput: %s", image, err, out)
	}

	containerId := strings.TrimSpace(string(out[:12]))
	return Id(containerId), nil
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
	client := http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	url := strings.TrimRight(serverUrl, "/") + "/health"

	var lastErr error

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err != nil {
			lastErr = err
			time.Sleep(200 * time.Millisecond)
			continue
		}

		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
			time.Sleep(200 * time.Millisecond)
			continue
		}
		return nil
	}
	if lastErr != nil {
		return fmt.Errorf("server did not become ready in %s: last error: %v", timeout, lastErr)
	}
	return fmt.Errorf("server did not become ready in %s", timeout)
}
