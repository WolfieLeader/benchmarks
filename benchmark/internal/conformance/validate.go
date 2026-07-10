package conformance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

var (
	uuidRe     = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	objectIDRe = regexp.MustCompile(`^[0-9a-fA-F]{24}$`)
)

// sha256ChainSeed is the fixed seed for the $sha256chain matcher: the runner
// applies SHA-256 to it n times and expects the server's /compute chain to
// match. It is contract canon and must equal every server's compute seed.
const sha256ChainSeed = "benchmark"

// sha256ChainPrefix flags the $sha256chain:<n> matcher token.
const sha256ChainPrefix = "$sha256chain:"

// validate checks status, headers, and body (text, htmlContains, or strict JSON
// with matchers). jwtSecret verifies $jwt tokens cryptographically.
func validate(exp *Expect, resp *http.Response, body, jwtSecret []byte) error {
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

	if len(exp.HTMLContains) > 0 {
		s := string(body)
		for _, want := range exp.HTMLContains {
			if !strings.Contains(s, want) {
				return fmt.Errorf("html body: missing expected substring %q (got: %s)", want, truncate(body, 300))
			}
		}
		return nil
	}

	if exp.Body != nil {
		var actual any
		// Duplicate keys in a response keep json/v1's last-wins tolerance so the
		// referee's judgment is unchanged by the v2 move.
		if err := json.Unmarshal(body, &actual, jsontext.AllowDuplicateNames(true)); err != nil {
			return fmt.Errorf("parse response JSON: %w (body: %s)", err, truncate(body, 200))
		}
		strict := exp.Match != "subset"
		if err := matchJSON(exp.Body, actual, strict, jwtSecret); err != nil {
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
// tokens ($uuid, $objectid, $id, $string, $number, $bool, $present, $absent,
// $optional, $jwt, $sha256chain:<n>) are recognized; every other value is
// compared exactly. When strict is true, object comparison rejects unexpected
// keys — but an expected $optional key never counts as unexpected (it is in
// want) and its absence is never missing. jwtSecret backs the $jwt matcher.
func matchJSON(want, got any, strict bool, jwtSecret []byte) error {
	switch w := want.(type) {
	case string:
		return matchScalarToken(w, got, jwtSecret)
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return fmt.Errorf("expected object, got %T", got)
		}
		for k, wv := range w {
			if s, isStr := wv.(string); isStr {
				switch s {
				case "$absent":
					if _, exists := g[k]; exists {
						return fmt.Errorf("key %q: expected absent, but present", k)
					}
					continue
				case "$optional":
					// The key MAY be absent; if present, any non-null value passes.
					gv, exists := g[k]
					if !exists {
						continue
					}
					if gv == nil {
						return fmt.Errorf("key %q: $optional present but null", k)
					}
					continue
				}
			}
			gv, exists := g[k]
			if !exists {
				return fmt.Errorf("key %q: missing", k)
			}
			if err := matchJSON(wv, gv, strict, jwtSecret); err != nil {
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
			if err := matchJSON(w[i], g[i], strict, jwtSecret); err != nil {
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

func matchScalarToken(want string, got any, jwtSecret []byte) error {
	switch want {
	case "$present":
		return nil
	case "$absent":
		return errors.New("$absent used as a value (only valid as an object key)")
	case "$optional":
		return errors.New("$optional used as a value (only valid as an object key)")
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
	case "$jwt":
		return matchJWT(got, jwtSecret)
	default:
		if rounds, ok := strings.CutPrefix(want, sha256ChainPrefix); ok {
			return matchSHA256Chain(want, rounds, got)
		}
		if s, ok := got.(string); !ok || s != want {
			return fmt.Errorf("got %v, want %q", got, want)
		}
		return nil
	}
}

// matchJWT verifies got is an HS256 JWT with a valid signature under jwtSecret
// and an unexpired, present exp claim — structural + cryptographic verification
// via golang-jwt (popular production lib over hand-rolled base64/HMAC). Claim
// values are asserted separately by the /jwt/verify step's strict body match.
func matchJWT(got any, jwtSecret []byte) error {
	s, ok := got.(string)
	if !ok {
		return fmt.Errorf("expected JWT string, got %T", got)
	}
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if _, err := parser.Parse(s, func(*jwt.Token) (any, error) { return jwtSecret, nil }); err != nil {
		return fmt.Errorf("JWT failed HS256 verification against the shared secret: %w", err)
	}
	return nil
}

// matchSHA256Chain recomputes the canonical SHA-256 chain (seed applied n times,
// lowercase hex) and compares it to got. The rounds count is carried in the
// matcher token ($sha256chain:<n>) so the runner needs no request context.
func matchSHA256Chain(token, rounds string, got any) error {
	n, err := strconv.Atoi(rounds)
	if err != nil || n < 0 {
		return fmt.Errorf("invalid matcher %q: rounds must be a non-negative integer", token)
	}
	s, ok := got.(string)
	if !ok {
		return fmt.Errorf("expected sha256 hex string, got %T", got)
	}
	if want := sha256Chain(n); s != want {
		return fmt.Errorf("sha256 chain (n=%d) mismatch: got %q, want %q", n, s, want)
	}
	return nil
}

// sha256Chain returns the lowercase hex of SHA-256 applied n times to the canon
// seed (n=0 is the raw seed bytes). It must match every server's /compute impl.
func sha256Chain(n int) string {
	state := []byte(sha256ChainSeed)
	for range n {
		sum := sha256.Sum256(state)
		state = sum[:]
	}
	return hex.EncodeToString(state)
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
