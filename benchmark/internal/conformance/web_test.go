package conformance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// These tests prove the web suite is not vacuously green: the same contract/web.json
// cases the harness runs PASS against a correct in-process stub and FAIL against
// stubs with a specific defect (wrong JWT secret, a verifier that ignores exp,
// wrong compute hash, a validator that accepts bad input, a template missing an
// interpolated value, a wrong 401 error string). If the referee (validate.go /
// the $jwt & $sha256chain matchers) were toothless, the broken-stub cases would
// still pass — so these guard it.

const (
	// Canon claims the /jwt/sign endpoint bakes (fixed) plus iat/exp (dynamic).
	// Asserted by contract/web.json's jwt flow verify step.
	claimSub  = "1234567890"
	claimName = "John Doe"
)

// computeCap is the canon /compute round cap: n above it is clamped, not rejected.
const computeCap = 1_000_000

var (
	emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	roleSet = []string{"admin", "user", "guest"}
)

// stubDefect selects a single wrong behavior for a broken stub; the zero value
// (all false) is the correct server.
type stubDefect struct {
	wrongJWTSecret bool // sign with a different secret -> $jwt signature check must fail
	ignoreExp      bool // verify the signature but skip exp validation -> the expired-token 401 case must fail
	wrongCompute   bool // return the wrong SHA-256 chain -> $sha256chain must fail
	acceptInvalid  bool // return 200 {"valid":true} for invalid input -> the fail case must fail
	htmlMissing    bool // drop an interpolated list item -> htmlContains must fail
	wrong401Error  bool // wrong error string on a bad token -> the 401 body match must fail
}

func newStub(defect stubDefect) *httptest.Server {
	secret := []byte(DefaultJWTSecret)
	if defect.wrongJWTSecret {
		secret = []byte("a-different-secret-than-the-canonical-one")
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /html", func(w http.ResponseWriter, _ *http.Request) {
		items := "<li>apple</li><li>banana</li><li>cherry</li>"
		if defect.htmlMissing {
			items = "<li>apple</li><li>banana</li>"
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!DOCTYPE html>\n<html><head><title>Benchmark</title></head>\n<body>\n" +
			"<h1>Hello, Alice</h1>\n<ul>" + items + "</ul>\n<p>Total: 42</p>\n</body></html>\n"))
	})

	mux.HandleFunc("GET /jwt/sign", func(w http.ResponseWriter, _ *http.Request) {
		now := time.Now()
		claims := jwt.MapClaims{
			"sub":   claimSub,
			"name":  claimName,
			"admin": true,
			"iat":   now.Unix(),
			"exp":   now.Add(time.Hour).Unix(),
		}
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"token": token})
	})

	mux.HandleFunc("GET /jwt/verify", func(w http.ResponseWriter, r *http.Request) {
		raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			writeInvalidToken(w, defect)
			return
		}
		parserOpts := []jwt.ParserOption{jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired()}
		if defect.ignoreExp {
			// The defect: signature is still verified, but claims validation
			// (including exp) is skipped — an expired token wrongly passes.
			parserOpts = []jwt.ParserOption{jwt.WithValidMethods([]string{"HS256"}), jwt.WithoutClaimsValidation()}
		}
		parser := jwt.NewParser(parserOpts...)
		claims := jwt.MapClaims{}
		if _, err := parser.ParseWithClaims(raw, claims, func(*jwt.Token) (any, error) { return secret, nil }); err != nil {
			writeInvalidToken(w, defect)
			return
		}
		writeJSON(w, http.StatusOK, claims)
	})

	mux.HandleFunc("POST /validate", func(w http.ResponseWriter, r *http.Request) {
		count := countValidationErrors(r)
		if count > 0 && !defect.acceptInvalid {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error":   "validation failed",
				"details": strconv.Itoa(count) + " error(s)",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"valid": true})
	})

	mux.HandleFunc("GET /compute", func(w http.ResponseWriter, r *http.Request) {
		// Canon: n must be an integer >= 1; missing/non-numeric/zero/negative -> 400.
		n, err := strconv.Atoi(r.URL.Query().Get("n"))
		if err != nil || n < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error":   "invalid n",
				"details": "n must be an integer >= 1",
			})
			return
		}
		if n > computeCap {
			n = computeCap
		}
		state := []byte(sha256ChainSeed)
		for range n {
			sum := sha256.Sum256(state)
			state = sum[:]
		}
		result := hex.EncodeToString(state)
		if defect.wrongCompute {
			result = strings.Repeat("0", len(result))
		}
		writeJSON(w, http.StatusOK, map[string]any{"result": result})
	})

	return httptest.NewServer(mux)
}

func writeInvalidToken(w http.ResponseWriter, defect stubDefect) {
	msg := "invalid token"
	if defect.wrong401Error {
		msg = "unauthorized"
	}
	writeJSON(w, http.StatusUnauthorized, map[string]any{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.MarshalWrite(w, body)
}

// countValidationErrors is a faithful (if partial) validator over the canon
// /validate schema: enough rules to correctly classify the pass/fail payloads.
func countValidationErrors(r *http.Request) int {
	var p struct {
		User struct {
			ID      string `json:"id"`
			Email   string `json:"email"`
			Profile struct {
				Age  *int   `json:"age"`
				Role string `json:"role"`
			} `json:"profile"`
		} `json:"user"`
	}
	if err := json.UnmarshalRead(r.Body, &p); err != nil {
		return 1
	}
	count := 0
	if !uuidRe.MatchString(p.User.ID) {
		count++
	}
	if !emailRe.MatchString(p.User.Email) {
		count++
	}
	if p.User.Profile.Age == nil || *p.User.Profile.Age < 0 || *p.User.Profile.Age > 120 {
		count++
	}
	if !slices.Contains(roleSet, p.User.Profile.Role) {
		count++
	}
	return count
}

// loadWebSuite loads the real contract/ dir and returns the web suite.
func loadWebSuite(t *testing.T) Suite {
	t.Helper()
	// Test cwd is this package dir (benchmark/internal/conformance); the contract
	// lives at the repo root, three levels up.
	suites, err := loadSuites(filepath.Join("..", "..", "..", "contract"))
	if err != nil {
		t.Fatalf("loadSuites: %v", err)
	}
	for _, s := range suites {
		if s.Name == "web" {
			return s
		}
	}
	t.Fatal("no suite named \"web\" found in contract/")
	return Suite{}
}

func runWebAgainst(t *testing.T, suite Suite, baseURL string) (passed, failed int, failures []failure) {
	t.Helper()
	hc := &http.Client{Timeout: requestTimeout}
	return runSuites(context.Background(), hc, baseURL, "", []Suite{suite}, nil, []byte(DefaultJWTSecret))
}

// TestWebSuitePassesAgainstCorrectStub proves every web case passes against a
// correct server — otherwise the suite could never be satisfied (vacuous-fail).
func TestWebSuitePassesAgainstCorrectStub(t *testing.T) {
	suite := loadWebSuite(t)
	srv := newStub(stubDefect{})
	defer srv.Close()

	passed, failed, failures := runWebAgainst(t, suite, srv.URL)
	if failed != 0 {
		for _, f := range failures {
			t.Errorf("unexpected failure %s/%s: %v", f.suite, f.name, f.err)
		}
	}
	if passed == 0 {
		t.Fatal("no web cases executed — suite is empty or misnamed")
	}
}

// TestSkipSuiteExcludesWeb proves the per-server gating: with the web suite in
// skipSuites, zero web cases execute even against a server that does NOT implement
// the web routes — so the 17 non-web servers' contract runs are unaffected by
// web.json's mere presence. (An httptest default mux 404s everything.)
func TestSkipSuiteExcludesWeb(t *testing.T) {
	suite := loadWebSuite(t)
	srv := httptest.NewServer(http.NewServeMux()) // implements nothing -> 404s
	defer srv.Close()

	hc := &http.Client{Timeout: requestTimeout}
	passed, failed, _ := runSuites(context.Background(), hc, srv.URL, "", []Suite{suite}, []string{"web"}, []byte(DefaultJWTSecret))
	if passed != 0 || failed != 0 {
		t.Fatalf("skip-suite=web must execute 0 web cases, ran passed=%d failed=%d", passed, failed)
	}

	// Sanity: without the skip, the same empty server fails the web cases — proving
	// the zero above is the skip working, not an empty suite.
	if _, failedNoSkip, _ := runSuites(context.Background(), hc, srv.URL, "", []Suite{suite}, nil, []byte(DefaultJWTSecret)); failedNoSkip == 0 {
		t.Fatal("without skip, web cases should fail against a server that implements nothing")
	}
}

// TestWebSuiteCatchesBrokenStubs proves the referee has teeth: each defect must
// surface as at least one failing case whose label mentions the expected route.
func TestWebSuiteCatchesBrokenStubs(t *testing.T) {
	suite := loadWebSuite(t)
	cases := []struct {
		name       string
		defect     stubDefect
		wantSubstr string // a failing case label must contain this
	}{
		{"wrong jwt secret", stubDefect{wrongJWTSecret: true}, "jwt"},
		{"verifies signature but ignores exp", stubDefect{ignoreExp: true}, "expired"},
		{"wrong compute hash", stubDefect{wrongCompute: true}, "compute"},
		{"validator accepts bad input", stubDefect{acceptInvalid: true}, "validate"},
		{"html missing interpolation", stubDefect{htmlMissing: true}, "html"},
		{"wrong 401 error string", stubDefect{wrong401Error: true}, "jwt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newStub(tc.defect)
			defer srv.Close()

			_, failed, failures := runWebAgainst(t, suite, srv.URL)
			if failed == 0 {
				t.Fatalf("defect %q produced no failures — the referee is toothless", tc.name)
			}
			if !slices.ContainsFunc(failures, func(f failure) bool {
				return strings.Contains(strings.ToLower(f.name), tc.wantSubstr)
			}) {
				var got []string
				for _, f := range failures {
					got = append(got, f.name)
				}
				t.Fatalf("defect %q: no failing case matched %q; failures: %v", tc.name, tc.wantSubstr, got)
			}
		})
	}
}
