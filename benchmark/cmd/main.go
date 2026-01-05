package main

import (
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"time"
)

var serverImages = []string{
	"go-chi-image",
	"go-fiber-image",
	"go-gin-image",
}

func main() {
	for _, img := range serverImages {
		containerId, err := startContainer(img)
		if err != nil {
			fmt.Printf("Failed to start container for image %s: %v\n", img, err)
			continue
		}

		if err := waitForServerReady(); err != nil {
			fmt.Printf("Server for image %s did not become ready: %v\n", img, err)
			killContainer(containerId)
			continue
		}

		killContainer(containerId)

		time.Sleep(1 * time.Second)
	}
}

func startContainer(image string) (string, error) {
	cmd := exec.Command("docker", "run", "-d", "--rm", "-p", "8080:8080", image)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	containerId := string(out[:12])
	return containerId, nil
}

func killContainer(containerId string) {
	cmd := exec.Command("docker", "kill", containerId)
	cmd.Run()
}

func waitForServerReady() error {
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
