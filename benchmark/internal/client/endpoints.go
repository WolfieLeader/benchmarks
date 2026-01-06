package client

import (
	"context"
	"encoding/json/v2"
	"io"
	"net/http"
	"time"
)

type helloWorldRes struct {
	Message string `json:"message"`
}

func (c *Client) helloWorld() (time.Duration, bool) {
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", c.serverUrl+"/", nil)
	if err != nil {
		return 0, false
	}

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB
	if err != nil {
		return 0, false
	}
	duration := time.Since(start)

	if resp.StatusCode != http.StatusOK {
		return duration, false
	}

	var data helloWorldRes
	if err := json.Unmarshal(body, &data); err != nil {
		return duration, false
	}

	if data.Message != "Hello, World!" {
		return duration, false
	}

	return duration, true
}
