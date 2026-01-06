package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	serverUrl  *url.URL
	httpClient http.Client
	ctx        context.Context
}

func New(ctx context.Context, serverUrl string) *Client {
	base, err := url.Parse(serverUrl)
	if err != nil {
		panic(fmt.Sprintf("invalid server URL: %v", err))
	}
	return &Client{serverUrl: base, httpClient: http.Client{}, ctx: ctx}
}

var rootEndpoint = &Endpoint{
	Path:   "/",
	Method: "GET",
	Expected: &Expected{
		StatusCode: 200,
		Body:       map[string]any{"message": "Hello, World!"},
	},
}

func (c *Client) RunBenchmarks() {
	const n = 1000
	durations := make([]time.Duration, 0, n)

	for range n {
		if dur, ok := c.testEndpoint(rootEndpoint, 5*time.Second); ok {
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
