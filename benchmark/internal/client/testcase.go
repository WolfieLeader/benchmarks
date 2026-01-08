package client

import (
	"benchmark-client/internal/utils"
	"cmp"
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/textproto"
	"net/url"
	"slices"
	"strings"
	"time"
)

type testcase struct {
	Url             string
	Method          string
	Headers         map[string]string
	Body            string
	StatusCode      int
	ExpectedHeaders map[string]string
	ExpectedBody    map[string]any
}

func (c *Client) CreateTestcasesFromEndpoint(endpoint *Endpoint) ([]*testcase, error) {
	method := strings.ToUpper(strings.TrimSpace(endpoint.Method))
	if !slices.Contains(methods, method) {
		return nil, fmt.Errorf("invalid method")
	}

	testcases := make([]*testcase, 0, len(endpoint.Testcases))
	for _, tc := range endpoint.Testcases {
		if tc == nil ||
			tc.StatusCode < 100 ||
			tc.StatusCode > 599 ||
			tc.Body == nil {
			continue
		}

		path, err := url.Parse(strings.TrimSpace(cmp.Or(tc.Path, endpoint.Path)))
		if err != nil {
			return nil, fmt.Errorf("invalid testcase path: %v", err)
		}

		headers := make(map[string]string, len(endpoint.Headers)+len(tc.OverrideHeaders))
		headersMapFn(endpoint.Headers, func(k, v string) { headers[k] = v })
		headersMapFn(tc.OverrideHeaders, func(k, v string) { headers[k] = v })

		body := make(map[string]any, len(endpoint.Body)+len(tc.OverrideBody))
		maps.Copy(body, endpoint.Body)
		maps.Copy(body, tc.OverrideBody)

		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("invalid testcase body: %v", err)
		}

		expectedHeaders := make(map[string]string, len(tc.Headers))
		headersMapFn(tc.Headers, func(k, v string) { expectedHeaders[k] = v })

		testcases = append(testcases,
			&testcase{
				Url:             c.serverUrl.ResolveReference(path).String(),
				Method:          method,
				Headers:         headers,
				Body:            string(bodyBytes),
				StatusCode:      tc.StatusCode,
				ExpectedHeaders: expectedHeaders,
				ExpectedBody:    tc.Body,
			})
	}

	if len(testcases) == 0 {
		return nil, fmt.Errorf("no valid testcases")
	}

	return testcases, nil
}

func headersMapFn(m map[string]string, fn func(key, value string)) {
	for k, v := range m {
		key := textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		fn(key, strings.TrimSpace(v))
	}
}

func (c *Client) testcase(tc *testcase) (time.Duration, error) {
	var bodyReader io.Reader
	if tc.Body != "" {
		bodyReader = strings.NewReader(tc.Body)
	}

	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, tc.Method, tc.Url, bodyReader)
	if err != nil {
		return 0, err
	}

	for key, value := range tc.Headers {
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
		return 0, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB
	if err != nil {
		return 0, err
	}

	latency := time.Since(start)

	if resp.StatusCode != tc.StatusCode {
		return latency, fmt.Errorf("unexpected status code: got %d, want %d", resp.StatusCode, tc.StatusCode)
	}

	for key, expectedValue := range tc.ExpectedHeaders {
		actualValue := strings.TrimSpace(resp.Header.Get(key))

		if strings.EqualFold(key, "Content-Type") {
			if !strings.Contains(actualValue, expectedValue) {
				return latency, fmt.Errorf("unexpected header %s: got %s, want %s", key, actualValue, expectedValue)
			}
			continue
		}

		if actualValue != expectedValue {
			return latency, fmt.Errorf("unexpected header %s: got %s, want %s", key, actualValue, expectedValue)
		}
	}

	if tc.ExpectedBody != nil {
		var actualBody map[string]any
		if err := json.Unmarshal(bodyBytes, &actualBody); err != nil {
			return latency, fmt.Errorf("invalid response body: %v", err)
		}

		if !utils.JsonMatch(tc.ExpectedBody, actualBody) {
			return latency, fmt.Errorf("unexpected body: got %v, want %v", actualBody, tc.ExpectedBody)
		}
	}

	return latency, nil
}
