package client

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	serverUrl  string
	httpClient http.Client
	ctx        context.Context
}

func New(ctx context.Context, serverUrl string) *Client {
	return &Client{serverUrl: strings.TrimRight(serverUrl, "/"), httpClient: http.Client{}, ctx: ctx}
}

type Result struct {
	Success  bool
	Duration time.Duration
}

func (c *Client) RunBenchmarks() {
	const n = 1000
	durations := make([]time.Duration, 0, n)

	for range n {
		if dur, ok := c.helloWorld(); ok {
			durations = append(durations, dur)
		}
	}

	avg := time.Duration(0)
	for _, dur := range durations {
		avg += dur
	}
	if len(durations) > 0 {
		avg /= time.Duration(len(durations))
	}

	fmt.Printf("Completed %d/%d requests successfully. Average duration: %s\n\n", len(durations), n, avg)
}
