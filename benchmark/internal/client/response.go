package client

import (
	"benchmark-client/internal/config"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// ValidateResponse validates the HTTP response against expected values
func ValidateResponse(tc *config.Testcase, resp *http.Response, body []byte) error {
	if resp.StatusCode != tc.ExpectedStatus {
		return fmt.Errorf("unexpected status code: got %d, want %d (body: %s)",
			resp.StatusCode, tc.ExpectedStatus, truncate(body, 200))
	}

	for key, expectedValue := range tc.ExpectedHeaders {
		actualValue := strings.TrimSpace(resp.Header.Get(key))

		// Content-Type gets substring match
		if strings.EqualFold(key, "Content-Type") {
			if !strings.Contains(actualValue, expectedValue) {
				return fmt.Errorf("unexpected header %s: got %q, want substring %q",
					key, actualValue, expectedValue)
			}
			continue
		}

		if actualValue != expectedValue {
			return fmt.Errorf("unexpected header %s: got %q, want %q",
				key, actualValue, expectedValue)
		}
	}

	if tc.ExpectedBody != nil {
		if err := validateJSONBody(tc.ExpectedBody, body); err != nil {
			return err
		}
	} else if tc.ExpectedText != "" {
		if err := validateTextBody(tc.ExpectedText, body); err != nil {
			return err
		}
	}

	return nil
}

func validateJSONBody(expected any, actual []byte) error {
	switch exp := expected.(type) {
	case map[string]any:
		var actualBody map[string]any
		if err := json.Unmarshal(actual, &actualBody); err != nil {
			return fmt.Errorf("failed to parse response as JSON object: %w (body: %s)",
				err, truncate(actual, 200))
		}
		if !jsonMatch(exp, actualBody) {
			return fmt.Errorf("JSON body mismatch: got %s, want %v",
				truncate(actual, 500), exp)
		}

	case []any:
		var actualBody []any
		if err := json.Unmarshal(actual, &actualBody); err != nil {
			return fmt.Errorf("failed to parse response as JSON array: %w (body: %s)",
				err, truncate(actual, 200))
		}
		if !jsonMatch(exp, actualBody) {
			return fmt.Errorf("JSON array mismatch: got %s, want %v",
				truncate(actual, 500), exp)
		}

	default:
		var actualValue any
		if err := json.Unmarshal(actual, &actualValue); err != nil {
			return fmt.Errorf("failed to parse response as JSON: %w (body: %s)",
				err, truncate(actual, 200))
		}
		if !jsonMatch(exp, actualValue) {
			return fmt.Errorf("JSON value mismatch: got %v, want %v", actualValue, exp)
		}
	}

	return nil
}

func validateTextBody(expected string, actual []byte) error {
	actualText := strings.TrimSpace(string(actual))
	expectedText := strings.TrimSpace(expected)

	if actualText != expectedText {
		return fmt.Errorf("text body mismatch: got %q, want %q",
			truncate([]byte(actualText), 200), expectedText)
	}
	return nil
}

func truncate(body []byte, maxLen int) string {
	if len(body) <= maxLen {
		return string(body)
	}
	return string(body[:maxLen]) + "..."
}

// jsonMatch checks if expected is a subset of got (for objects)
// or an exact match (for arrays and primitives)
func jsonMatch(want, got any) bool {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return false
		}
		for k, wv := range w {
			gv, exists := g[k]
			if !exists || !jsonMatch(wv, gv) {
				return false
			}
		}
		return true

	case []any:
		g, ok := got.([]any)
		if !ok || len(w) != len(g) {
			return false
		}
		for i := range w {
			if !jsonMatch(w[i], g[i]) {
				return false
			}
		}
		return true

	default:
		return want == got
	}
}
