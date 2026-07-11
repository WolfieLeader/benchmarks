package routes

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

const testSecret = "benchmarks-shared-jwt-secret-dev-default"

// These are the exact static tokens from contract/web.json — the bad-signature
// token is signed with a throwaway secret, the expired token with the dev-default
// secret but exp in 2020. Both must be rejected under testSecret.
const (
	badSigToken  = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhZG1pbiI6dHJ1ZSwiZXhwIjo0MTAyNDQ0ODAwLCJpYXQiOjE3MzU2ODk2MDAsIm5hbWUiOiJKb2huIERvZSIsInN1YiI6IjEyMzQ1Njc4OTAifQ.J75FiSXpAhQxN9jiUjBHADeu_su1WJnZjJqDXI4aOWw"
	expiredToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhZG1pbiI6dHJ1ZSwiZXhwIjoxNTc3ODQwNDAwLCJpYXQiOjE1Nzc4MzY4MDAsIm5hbWUiOiJKb2huIERvZSIsInN1YiI6IjEyMzQ1Njc4OTAifQ.8XxPN0yJufkzy8TdEspyV-GqR1b1MF8aW_YVERdoRic"
)

func webRouter() chi.Router {
	r := chi.NewRouter()
	RegisterWeb(r, testSecret)
	return r
}

func do(t *testing.T, h http.Handler, req *http.Request) (*httptest.ResponseRecorder, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, rec.Body.String()
}

func sha256Chain(n int) string {
	state := []byte("benchmark")
	for range n {
		sum := sha256.Sum256(state)
		state = sum[:]
	}
	return hex.EncodeToString(state)
}

func TestHTML(t *testing.T) {
	rec, body := do(t, webRouter(), httptest.NewRequest(http.MethodGet, "/html", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type = %q, want text/html", ct)
	}
	for _, want := range []string{"Hello, Alice", "apple", "banana", "cherry", "Total: 42"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
}

func TestCompute(t *testing.T) {
	r := webRouter()
	cases := []struct {
		name   string
		query  string
		status int
		result string
	}{
		{"one_round", "n=1", 200, sha256Chain(1)},
		{"thousand_rounds", "n=1000", 200, sha256Chain(1000)},
		{"clamped", "n=5000000", 200, sha256Chain(1_000_000)},
		{"missing", "", 400, ""},
		{"non_numeric", "n=abc", 400, ""},
		{"zero", "n=0", 400, ""},
		{"negative", "n=-5", 400, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, body := do(t, r, httptest.NewRequest(http.MethodGet, "/compute?"+tc.query, nil))
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.status, body)
			}
			if tc.status == 200 {
				var got map[string]string
				if err := json.Unmarshal([]byte(body), &got); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if got["result"] != tc.result {
					t.Fatalf("result = %q, want %q", got["result"], tc.result)
				}
			} else if !strings.Contains(body, `"invalid n"`) {
				t.Fatalf("400 body missing invalid n error: %s", body)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	valid := `{"user":{"id":"3f1a2b3c-4d5e-6f70-8192-a3b4c5d6e7f8","email":"alice@conformance-suite.com","profile":{"age":30,"role":"admin","preferences":{"theme":"dark","notifications":true}}},"items":[{"sku":"SKU-1","quantity":2,"tags":["new","featured"]},{"sku":"SKU-2","quantity":100,"tags":[]}],"total":42.5}`
	invalid := `{"user":{"id":"not-a-uuid","email":"not-an-email","profile":{"age":200,"role":"superuser","preferences":{"theme":"neon","notifications":true}}},"items":[{"sku":"SKU-1","quantity":0,"tags":["x"]}],"total":-5}`
	r := webRouter()

	rec, body := do(t, r, httptest.NewRequest(http.MethodPost, "/validate", strings.NewReader(valid)))
	if rec.Code != http.StatusOK {
		t.Fatalf("valid: status = %d, want 200 (body %s)", rec.Code, body)
	}
	if !strings.Contains(body, `"valid":true`) {
		t.Fatalf("valid: body = %s, want valid:true", body)
	}

	rec, body = do(t, r, httptest.NewRequest(http.MethodPost, "/validate", strings.NewReader(invalid)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid: status = %d, want 400 (body %s)", rec.Code, body)
	}
	if !strings.Contains(body, `"validation failed"`) {
		t.Fatalf("invalid: body = %s, want validation failed", body)
	}
}

func TestJWTSignThenVerify(t *testing.T) {
	r := webRouter()

	rec, body := do(t, r, httptest.NewRequest(http.MethodGet, "/jwt/sign", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("sign: status = %d, want 200", rec.Code)
	}
	var signed map[string]string
	if err := json.Unmarshal([]byte(body), &signed); err != nil {
		t.Fatalf("sign decode: %v", err)
	}
	token := signed["token"]
	if token == "" {
		t.Fatal("sign: empty token")
	}

	req := httptest.NewRequest(http.MethodGet, "/jwt/verify", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec, body = do(t, r, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("verify: status = %d, want 200 (body %s)", rec.Code, body)
	}
	var claims map[string]any
	if err := json.Unmarshal([]byte(body), &claims); err != nil {
		t.Fatalf("verify decode: %v", err)
	}
	if claims["sub"] != "1234567890" || claims["name"] != "John Doe" || claims["admin"] != true {
		t.Fatalf("verify claims = %v", claims)
	}
	if _, ok := claims["iat"].(float64); !ok {
		t.Fatalf("verify iat not a number: %v", claims["iat"])
	}
	if _, ok := claims["exp"].(float64); !ok {
		t.Fatalf("verify exp not a number: %v", claims["exp"])
	}
	if len(claims) != 5 {
		t.Fatalf("verify claims has %d keys, want exactly 5: %v", len(claims), claims)
	}
}

func TestJWTVerifyRejects(t *testing.T) {
	r := webRouter()
	cases := []struct {
		name string
		auth string
	}{
		{"missing", ""},
		{"malformed", "Bearer not-a-jwt"},
		{"bad_signature", "Bearer " + badSigToken},
		{"expired", "Bearer " + expiredToken},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/jwt/verify", nil)
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			rec, body := do(t, r, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401 (body %s)", rec.Code, body)
			}
			if !strings.Contains(body, `"invalid token"`) {
				t.Fatalf("body missing invalid token: %s", body)
			}
		})
	}
}
