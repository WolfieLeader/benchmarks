package container

import (
	"context"
	"fmt"
	"io"
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

func WaitToBeReady() error {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://localhost:8080/ping")
		if err == nil && resp.StatusCode == http.StatusOK {
			data, _ := io.ReadAll(resp.Body)
			fmt.Printf("Server response: %s\n", data)

			resp.Body.Close()
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("server did not become ready in time")
}
