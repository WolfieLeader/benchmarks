package client

import (
	"benchmark-client/internal/utils"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// ValidateResponse validates the HTTP response against expected values
func ValidateResponse(tc *ExecutableTestcase, resp *http.Response, body []byte) error {
	// Validate status code
	if resp.StatusCode != tc.ExpectedStatus {
		return fmt.Errorf("unexpected status code: got %d, want %d (body: %s)",
			resp.StatusCode, tc.ExpectedStatus, truncateBody(body, 200))
	}

	// Validate expected headers
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

	// Validate body based on expected type
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
	// Handle different expected types
	switch exp := expected.(type) {
	case map[string]any:
		var actualBody map[string]any
		if err := json.Unmarshal(actual, &actualBody); err != nil {
			return fmt.Errorf("failed to parse response as JSON object: %w (body: %s)",
				err, truncateBody(actual, 200))
		}
		if !utils.JsonMatch(exp, actualBody) {
			return fmt.Errorf("JSON body mismatch: got %s, want %v",
				truncateBody(actual, 500), exp)
		}

	case []any:
		var actualBody []any
		if err := json.Unmarshal(actual, &actualBody); err != nil {
			return fmt.Errorf("failed to parse response as JSON array: %w (body: %s)",
				err, truncateBody(actual, 200))
		}
		if !utils.JsonMatch(exp, actualBody) {
			return fmt.Errorf("JSON array mismatch: got %s, want %v",
				truncateBody(actual, 500), exp)
		}

	default:
		// For primitive types, try to unmarshal and compare
		var actualValue any
		if err := json.Unmarshal(actual, &actualValue); err != nil {
			return fmt.Errorf("failed to parse response as JSON: %w (body: %s)",
				err, truncateBody(actual, 200))
		}
		if !utils.JsonMatch(exp, actualValue) {
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
			truncateString(actualText, 200), expectedText)
	}
	return nil
}

func truncateBody(body []byte, maxLen int) string {
	return truncateString(string(body), maxLen)
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
