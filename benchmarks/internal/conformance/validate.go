package conformance

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"
)

var (
	uuidRe     = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	objectIDRe = regexp.MustCompile(`^[0-9a-fA-F]{24}$`)
)

// validate checks status, headers, and body (text or strict JSON with matchers).
func validate(exp *Expect, resp *http.Response, body []byte) error {
	if len(exp.StatusAnyOf) > 0 {
		if !slices.Contains(exp.StatusAnyOf, resp.StatusCode) {
			return fmt.Errorf("status: got %d, want one of %v (body: %s)", resp.StatusCode, exp.StatusAnyOf, truncate(body, 200))
		}
	} else if resp.StatusCode != exp.Status {
		return fmt.Errorf("status: got %d, want %d (body: %s)", resp.StatusCode, exp.Status, truncate(body, 200))
	}

	for key, want := range exp.Headers {
		if !headerContains(resp.Header, key, want) {
			return fmt.Errorf("header %s: got %q, want to contain %q", key, resp.Header.Get(key), want)
		}
	}

	if exp.Text != nil {
		got := strings.TrimSpace(string(body))
		want := strings.TrimSpace(*exp.Text)
		if got != want {
			return fmt.Errorf("text body: got %q, want %q", truncate([]byte(got), 200), want)
		}
		return nil
	}

	if exp.Body != nil {
		var actual any
		if err := json.Unmarshal(body, &actual); err != nil {
			return fmt.Errorf("parse response JSON: %w (body: %s)", err, truncate(body, 200))
		}
		strict := exp.Match != "subset"
		if err := matchJSON(exp.Body, actual, strict); err != nil {
			return fmt.Errorf("body: %w (got: %s)", err, truncate(body, 300))
		}
	}

	return nil
}

func headerContains(h http.Header, key, want string) bool {
	for _, v := range h.Values(http.CanonicalHeaderKey(key)) {
		if strings.Contains(v, want) {
			return true
		}
	}
	return false
}

// matchJSON compares an expected value against an actual value. String matcher
// tokens ($uuid, $objectid, $id, $string, $number, $bool, $present, $absent) are
// recognized; every other value is compared exactly. When strict is true, object
// comparison rejects unexpected keys.
func matchJSON(want, got any, strict bool) error {
	switch w := want.(type) {
	case string:
		return matchScalarToken(w, got)
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return fmt.Errorf("expected object, got %T", got)
		}
		for k, wv := range w {
			if s, isStr := wv.(string); isStr && s == "$absent" {
				if _, exists := g[k]; exists {
					return fmt.Errorf("key %q: expected absent, but present", k)
				}
				continue
			}
			gv, exists := g[k]
			if !exists {
				return fmt.Errorf("key %q: missing", k)
			}
			if err := matchJSON(wv, gv, strict); err != nil {
				return fmt.Errorf("key %q: %w", k, err)
			}
		}
		if strict {
			if extra := extraKeys(w, g); len(extra) > 0 {
				return fmt.Errorf("unexpected keys: %s", strings.Join(extra, ", "))
			}
		}
		return nil
	case []any:
		g, ok := got.([]any)
		if !ok {
			return fmt.Errorf("expected array, got %T", got)
		}
		if len(w) != len(g) {
			return fmt.Errorf("array length: got %d, want %d", len(g), len(w))
		}
		for i := range w {
			if err := matchJSON(w[i], g[i], strict); err != nil {
				return fmt.Errorf("[%d]: %w", i, err)
			}
		}
		return nil
	default:
		if want != got {
			return fmt.Errorf("got %v, want %v", got, want)
		}
		return nil
	}
}

func extraKeys(want, got map[string]any) []string {
	var extra []string
	for k := range got {
		if _, ok := want[k]; !ok {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	return extra
}

func matchScalarToken(want string, got any) error {
	switch want {
	case "$present":
		return nil
	case "$absent":
		return errors.New("$absent used as a value (only valid as an object key)")
	case "$string":
		if _, ok := got.(string); !ok {
			return fmt.Errorf("expected string, got %T", got)
		}
		return nil
	case "$number":
		if _, ok := got.(float64); !ok {
			return fmt.Errorf("expected number, got %T", got)
		}
		return nil
	case "$bool":
		if _, ok := got.(bool); !ok {
			return fmt.Errorf("expected bool, got %T", got)
		}
		return nil
	case "$uuid":
		return matchPattern(got, uuidRe, "UUID")
	case "$objectid":
		return matchPattern(got, objectIDRe, "ObjectId")
	case "$id":
		s, ok := got.(string)
		if !ok {
			return fmt.Errorf("expected id string, got %T", got)
		}
		if !uuidRe.MatchString(s) && !objectIDRe.MatchString(s) {
			return fmt.Errorf("%q is not a UUID or ObjectId", s)
		}
		return nil
	default:
		if s, ok := got.(string); !ok || s != want {
			return fmt.Errorf("got %v, want %q", got, want)
		}
		return nil
	}
}

func matchPattern(got any, re *regexp.Regexp, label string) error {
	s, ok := got.(string)
	if !ok {
		return fmt.Errorf("expected %s string, got %T", label, got)
	}
	if !re.MatchString(s) {
		return fmt.Errorf("%q is not a valid %s", s, label)
	}
	return nil
}

func truncate(body []byte, maxLen int) string {
	if len(body) <= maxLen {
		return string(body)
	}
	return string(body[:maxLen]) + "..."
}
