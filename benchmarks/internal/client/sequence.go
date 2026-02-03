package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"benchmark-client/internal/config"
)

type SequenceResult struct {
	SequenceId      string
	Database        string
	TotalDuration   time.Duration
	StepDurations   []time.Duration
	Success         bool
	FailedStep      int
	Error           string
	ContextCanceled bool
}

func RunSequence(ctx context.Context, client *http.Client, baseURL string, seq *config.ResolvedSequence, workerID, cycleNum int, timeout time.Duration) SequenceResult {
	result := SequenceResult{
		SequenceId:    seq.Id,
		Database:      seq.Database,
		StepDurations: make([]time.Duration, len(seq.Endpoints)),
		FailedStep:    -1,
		Success:       true,
	}

	vars := generateVars(seq.Vars, workerID, cycleNum)
	captured := make(map[string]string)

	var totalDuration time.Duration

	for i, endpoint := range seq.Endpoints {
		stepCtx, cancel := context.WithTimeout(ctx, timeout)
		stepDuration, err := executeSequenceStep(stepCtx, client, baseURL, endpoint, vars, captured)
		cancel()

		result.StepDurations[i] = stepDuration
		totalDuration += stepDuration

		if err != nil {
			result.Success = false
			result.FailedStep = i
			result.Error = err.Error()
			result.TotalDuration = totalDuration
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				result.ContextCanceled = true
			}
			return result
		}
	}

	result.TotalDuration = totalDuration
	return result
}

func generateVars(varDefs map[string]config.VarConfig, workerID, cycleNum int) map[string]any {
	vars := make(map[string]any)

	for name, cfg := range varDefs {
		if cfg.Optional != nil {
			skipProb := 0.5
			switch v := cfg.Optional.(type) {
			case bool:
				if v {
					skipProb = 0.5
				} else {
					skipProb = 0
				}
			case float64:
				skipProb = v
			}
			if rand.Float64() < skipProb { //nolint:gosec // test data generation, not security-sensitive
				vars[name] = nil // nil means omit the field
				continue
			}
		}

		switch cfg.Type {
		case "email":
			vars[name] = fmt.Sprintf("user-%d-%d@test.com", workerID, cycleNum)
		case "int":
			if cfg.Max > cfg.Min {
				vars[name] = rand.Intn(cfg.Max-cfg.Min+1) + cfg.Min //nolint:gosec // test data generation
			} else {
				vars[name] = cfg.Min
			}
		}
	}

	return vars
}

func executeSequenceStep(ctx context.Context, client *http.Client, baseURL string, endpoint *config.ResolvedSequenceEndpoint, vars map[string]any, captured map[string]string) (time.Duration, error) {
	path := replacePlaceholdersInString(endpoint.Path, vars, captured)
	url := baseURL + path

	var bodyReader io.Reader
	if endpoint.Body != nil {
		bodyWithVars := replacePlaceholdersInBody(endpoint.Body, vars, captured)
		bodyBytes, err := json.Marshal(bodyWithVars)
		if err != nil {
			return 0, fmt.Errorf("failed to marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, endpoint.Method, url, bodyReader)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	if endpoint.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range endpoint.Headers {
		req.Header.Set(k, v)
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return time.Since(start), fmt.Errorf("request failed: %w", err)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	closeErr := resp.Body.Close()
	duration := time.Since(start)

	if err != nil {
		return duration, fmt.Errorf("failed to read response: %w", err)
	}
	if closeErr != nil {
		return duration, closeErr
	}

	if resp.StatusCode != endpoint.ExpectedStatus {
		return duration, fmt.Errorf("status %d, want %d: %s", resp.StatusCode, endpoint.ExpectedStatus, truncate(body, 200))
	}

	var respData any
	needsParse := len(endpoint.Capture) > 0 || endpoint.ExpectedBody != nil
	if needsParse {
		if err := json.Unmarshal(body, &respData); err != nil {
			return duration, fmt.Errorf("failed to parse response: %w", err)
		}
	}

	if len(endpoint.Capture) > 0 {
		respMap, ok := respData.(map[string]any)
		if !ok {
			return duration, fmt.Errorf("expected JSON object for capture, got %T", respData)
		}
		for varName, fieldName := range endpoint.Capture {
			val, ok := respMap[fieldName]
			if !ok {
				return duration, fmt.Errorf("capture field %q not found in response", fieldName)
			}
			captured[varName] = anyToString(val)
		}
	}

	if endpoint.ExpectedBody != nil {
		expectedWithVars := replacePlaceholdersInBody(endpoint.ExpectedBody, vars, captured)
		if err := validatePartialMatch(expectedWithVars, respData); err != nil {
			return duration, fmt.Errorf("body validation failed: %w", err)
		}
	}

	return duration, nil
}

func replacePlaceholdersInString(s string, vars map[string]any, captured map[string]string) string {
	result := s
	for key, val := range vars {
		if val == nil {
			continue
		}
		result = strings.ReplaceAll(result, "{"+key+"}", anyToString(val))
	}
	for key, val := range captured {
		result = strings.ReplaceAll(result, "{"+key+"}", val)
	}
	return result
}

func replacePlaceholdersInBody(body any, vars map[string]any, captured map[string]string) any {
	switch v := body.(type) {
	case string:
		if strings.HasPrefix(v, "{") && strings.HasSuffix(v, "}") {
			varName := v[1 : len(v)-1]
			if val, ok := vars[varName]; ok {
				if val == nil {
					return nil // Signal to omit this field
				}
				return val
			}
		}
		return replacePlaceholdersInString(v, vars, captured)
	case map[string]any:
		result := make(map[string]any)
		for key, val := range v {
			replaced := replacePlaceholdersInBody(val, vars, captured)
			if replaced != nil { // Omit nil values (optional vars)
				result[key] = replaced
			}
		}
		return result
	case []any:
		result := make([]any, 0, len(v))
		for _, item := range v {
			replaced := replacePlaceholdersInBody(item, vars, captured)
			if replaced != nil {
				result = append(result, replaced)
			}
		}
		return result
	default:
		return body
	}
}

func validatePartialMatch(expected, actual any) error {
	if expected == nil {
		return nil
	}

	switch exp := expected.(type) {
	case map[string]any:
		act, ok := actual.(map[string]any)
		if !ok {
			return fmt.Errorf("expected object, got %T", actual)
		}
		for key, expVal := range exp {
			if expVal == nil {
				continue
			}
			actVal, ok := act[key]
			if !ok {
				return fmt.Errorf("missing field %q", key)
			}
			if err := validatePartialMatch(expVal, actVal); err != nil {
				return fmt.Errorf("field %q: %w", key, err)
			}
		}
	case []any:
		act, ok := actual.([]any)
		if !ok {
			return fmt.Errorf("expected array, got %T", actual)
		}
		if len(exp) != len(act) {
			return fmt.Errorf("array length %d, want %d", len(act), len(exp))
		}
		for i := range exp {
			if err := validatePartialMatch(exp[i], act[i]); err != nil {
				return fmt.Errorf("[%d]: %w", i, err)
			}
		}
	default:
		if !reflect.DeepEqual(expected, actual) {
			return fmt.Errorf("got %v, want %v", actual, expected)
		}
	}
	return nil
}

func anyToString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return strconv.FormatInt(int64(val), 10)
		}
		return strconv.FormatFloat(val, 'f', -1, 64)
	case int:
		return strconv.Itoa(val)
	default:
		return fmt.Sprintf("%v", v)
	}
}
