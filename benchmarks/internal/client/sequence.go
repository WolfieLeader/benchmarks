package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// SequenceStep defines one step in a sequence
type SequenceStep struct {
	Method         string         `json:"method"`
	Path           string         `json:"path"`
	Body           map[string]any `json:"body,omitempty"`
	ExpectedStatus int            `json:"expected_status"`
	Capture        string         `json:"capture,omitempty"`
}

// SequenceEndpoint defines a multi-step test sequence
type SequenceEndpoint struct {
	Name      string              `json:"name"`
	Type      string              `json:"type"`
	Steps     []SequenceStep      `json:"steps"`
	TestCases []map[string]string `json:"test_cases"`
}

// SequenceResult contains the results of executing a sequence
type SequenceResult struct {
	TotalDuration time.Duration
	StepDurations []time.Duration
	Success       bool
	FailedStep    int
	Error         string
}

// RunSequence executes one full sequence cycle.
// Returns total cycle duration and per-step durations.
func RunSequence(client *http.Client, baseURL string, seq *SequenceEndpoint, testCase map[string]string, workerID, cycleNum int) (SequenceResult, error) {
	result := SequenceResult{
		StepDurations: make([]time.Duration, len(seq.Steps)),
		FailedStep:    -1,
		Success:       true,
	}

	captured := make(map[string]string)
	var totalDuration time.Duration

	for i, step := range seq.Steps {
		stepDuration, capturedValue, err := executeStep(client, baseURL, step, testCase, captured, workerID, cycleNum)
		result.StepDurations[i] = stepDuration
		totalDuration += stepDuration

		if err != nil {
			result.Success = false
			result.FailedStep = i
			result.Error = err.Error()
			result.TotalDuration = totalDuration
			return result, err
		}

		if step.Capture != "" && capturedValue != "" {
			captured[step.Capture] = capturedValue
		}
	}

	result.TotalDuration = totalDuration
	return result, nil
}

func executeStep(client *http.Client, baseURL string, step SequenceStep, testCase map[string]string, captured map[string]string, workerID, cycleNum int) (time.Duration, string, error) {
	path := replacePlaceholders(step.Path, testCase, captured, workerID, cycleNum)
	url := baseURL + path

	var bodyReader io.Reader
	if step.Body != nil {
		bodyWithPlaceholders := replaceBodyPlaceholders(step.Body, testCase, captured, workerID, cycleNum)
		bodyBytes, err := json.Marshal(bodyWithPlaceholders)
		if err != nil {
			return 0, "", fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, step.Method, url, bodyReader)
	if err != nil {
		return 0, "", fmt.Errorf("failed to create request: %w", err)
	}

	if step.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return time.Since(start), "", fmt.Errorf("request failed: %w", err)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	closeErr := resp.Body.Close()
	duration := time.Since(start)

	if err != nil {
		return duration, "", fmt.Errorf("failed to read response: %w", err)
	}
	if closeErr != nil {
		return duration, "", fmt.Errorf("failed to close response: %w", closeErr)
	}

	if resp.StatusCode != step.ExpectedStatus {
		return duration, "", fmt.Errorf("unexpected status code: got %d, want %d (body: %s)",
			resp.StatusCode, step.ExpectedStatus, truncateBody(body, 200))
	}

	var capturedValue string
	if step.Capture != "" {
		capturedValue, err = extractField(body, step.Capture)
		if err != nil {
			return duration, "", fmt.Errorf("failed to capture field %q: %w", step.Capture, err)
		}
	}

	return duration, capturedValue, nil
}

func replacePlaceholders(path string, testCase map[string]string, captured map[string]string, workerID, cycleNum int) string {
	result := path

	for key, value := range testCase {
		result = strings.ReplaceAll(result, "{"+key+"}", value)
	}

	for key, value := range captured {
		result = strings.ReplaceAll(result, "{"+key+"}", value)
	}

	uniqueEmail := fmt.Sprintf("user-%d-%d@test.com", workerID, cycleNum)
	result = strings.ReplaceAll(result, "{unique_email}", uniqueEmail)

	return result
}

func replaceBodyPlaceholders(body map[string]any, testCase map[string]string, captured map[string]string, workerID, cycleNum int) map[string]any {
	result := make(map[string]any, len(body))

	for key, value := range body {
		switch v := value.(type) {
		case string:
			result[key] = replaceStringPlaceholders(v, testCase, captured, workerID, cycleNum)
		case map[string]any:
			result[key] = replaceBodyPlaceholders(v, testCase, captured, workerID, cycleNum)
		default:
			result[key] = value
		}
	}

	return result
}

func replaceStringPlaceholders(s string, testCase map[string]string, captured map[string]string, workerID, cycleNum int) string {
	result := s

	for key, value := range testCase {
		result = strings.ReplaceAll(result, "{"+key+"}", value)
	}

	for key, value := range captured {
		result = strings.ReplaceAll(result, "{"+key+"}", value)
	}

	uniqueEmail := fmt.Sprintf("user-%d-%d@test.com", workerID, cycleNum)
	result = strings.ReplaceAll(result, "{unique_email}", uniqueEmail)

	return result
}

func extractField(body []byte, field string) (string, error) {
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return "", fmt.Errorf("failed to parse JSON: %w", err)
	}

	value, ok := data[field]
	if !ok {
		return "", fmt.Errorf("field %q not found in response", field)
	}

	switch v := value.(type) {
	case string:
		return v, nil
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10), nil
		}
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

func truncateBody(body []byte, maxLen int) string {
	if len(body) <= maxLen {
		return string(body)
	}
	return string(body[:maxLen]) + "..."
}
