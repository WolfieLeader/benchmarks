package client

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"io"
	"net/http"
	"net/textproto"
	"net/url"
	"slices"
	"strings"
	"time"
)

type Endpoint struct {
	Path     string
	Method   string
	Headers  map[string]string
	Body     map[string]any
	Expected *Expected
}

type Expected struct {
	StatusCode int
	Headers    map[string]string
	Body       map[string]any
}

var methods = []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}

func (c *Client) testEndpoint(endpoint *Endpoint, timeout time.Duration) (time.Duration, bool) {
	if endpoint == nil || endpoint.Path == "" || endpoint.Method == "" || endpoint.Expected == nil || len(endpoint.Expected.Body) == 0 || endpoint.Expected.StatusCode < 100 || endpoint.Expected.StatusCode > 599 {
		return 0, false
	}

	method := strings.ToUpper(strings.TrimSpace(endpoint.Method))
	if !slices.Contains(methods, method) {
		return 0, false
	}

	path, err := url.Parse(strings.TrimSpace(endpoint.Path))
	if err != nil {
		return 0, false
	}

	url := c.serverUrl.ResolveReference(path)

	var bodyReader io.Reader
	if len(endpoint.Body) > 0 {
		bodyBytes, err := json.Marshal(endpoint.Body)
		if err != nil {
			return 0, false
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	ctx, cancel := context.WithTimeout(c.ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, url.String(), bodyReader)
	if err != nil {
		return 0, false
	}

	for key, value := range endpoint.Headers {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			req.Header.Set(key, value)
		}
	}

	if bodyReader != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB
	if err != nil {
		return 0, false
	}

	latency := time.Since(start)

	if resp.StatusCode != endpoint.Expected.StatusCode {
		return latency, false
	}

	for key, expectedValue := range endpoint.Expected.Headers {
		key = textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(key))
		if key == "" {
			continue
		}

		expectedValue = strings.TrimSpace(expectedValue)
		actualValue := strings.TrimSpace(resp.Header.Get(key))

		if strings.EqualFold(key, "Content-Type") {
			if !strings.Contains(actualValue, expectedValue) {
				return latency, false
			}
			continue
		}

		if actualValue != expectedValue {
			return latency, false
		}
	}

	var actualBody any
	if err := json.Unmarshal(bodyBytes, &actualBody); err != nil {
		return latency, false
	}

	expectedBytes, err := json.Marshal(endpoint.Expected.Body)
	if err != nil {
		return latency, false
	}

	var expectedBody any
	if err := json.Unmarshal(expectedBytes, &expectedBody); err != nil {
		return latency, false
	}

	if !jsonContains(expectedBody, actualBody) {
		return latency, false
	}

	return latency, true
}

func jsonContains(want, got any) bool {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return false
		}
		for k, wv := range w {
			gv, exists := g[k]
			if !exists {
				return false
			}
			if !jsonContains(wv, gv) {
				return false
			}
		}
		return true

	case []any:
		g, ok := got.([]any)
		if !ok {
			return false
		}
		if len(w) != len(g) {
			return false
		}
		for i := range w {
			if !jsonContains(w[i], g[i]) {
				return false
			}
		}
		return true

	default:
		return want == got
	}
}
