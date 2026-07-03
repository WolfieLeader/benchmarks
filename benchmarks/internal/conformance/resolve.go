package conformance

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// resolveCase returns a copy of c with all {var} placeholders (in path, headers,
// query, bodies, and expectations) substituted from captured values.
func resolveCase(c *Case, captured map[string]string) Case {
	out := *c
	out.Path = substituteString(c.Path, captured)
	out.Headers = substituteMap(c.Headers, captured)
	out.Query = substituteMap(c.Query, captured)
	if c.RawBody != nil {
		s := substituteString(*c.RawBody, captured)
		out.RawBody = &s
	}
	out.Body = substituteAny(c.Body, captured)
	out.Expect = c.Expect
	out.Expect.Body = substituteAny(c.Expect.Body, captured)
	out.Expect.Headers = substituteMap(c.Expect.Headers, captured)
	if c.Expect.Text != nil {
		s := substituteString(*c.Expect.Text, captured)
		out.Expect.Text = &s
	}
	return out
}

func substituteString(s string, captured map[string]string) string {
	if !strings.Contains(s, "{") {
		return s
	}
	for k, v := range captured {
		s = strings.ReplaceAll(s, "{"+k+"}", v)
	}
	return s
}

func substituteMap(m map[string]string, captured map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = substituteString(v, captured)
	}
	return out
}

func substituteAny(v any, captured map[string]string) any {
	switch val := v.(type) {
	case string:
		return substituteString(val, captured)
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, item := range val {
			out[k] = substituteAny(item, captured)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = substituteAny(item, captured)
		}
		return out
	default:
		return v
	}
}

// capture extracts response fields into the captured map for later steps.
func capture(spec map[string]string, body []byte, captured map[string]string) error {
	if len(spec) == 0 {
		return nil
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("capture: response is not a JSON object: %w", err)
	}
	for varName, field := range spec {
		val, ok := parsed[field]
		if !ok {
			return fmt.Errorf("capture: field %q not found in response", field)
		}
		captured[varName] = scalarToString(val)
	}
	return nil
}

func scalarToString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return strconv.FormatInt(int64(val), 10)
		}
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(val)
	default:
		return fmt.Sprintf("%v", v)
	}
}
