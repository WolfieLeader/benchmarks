package routes

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

const testSecret = "benchmarks-shared-jwt-secret-dev-default"

// These are the exact static tokens from contract/web.json — the bad-signature
// token is signed with a throwaway secret, the expired token with the dev-default
// secret but exp in 2020. Both must be rejected under testSecret.
const (
	badSigToken  = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhZG1pbiI6dHJ1ZSwiZXhwIjo0MTAyNDQ0ODAwLCJpYXQiOjE3MzU2ODk2MDAsIm5hbWUiOiJKb2huIERvZSIsInN1YiI6IjEyMzQ1Njc4OTAifQ.J75FiSXpAhQxN9jiUjBHADeu_su1WJnZjJqDXI4aOWw"
	expiredToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhZG1pbiI6dHJ1ZSwiZXhwIjoxNTc3ODQwNDAwLCJpYXQiOjE1Nzc4MzY4MDAsIm5hbWUiOiJKb2huIERvZSIsInN1YiI6IjEyMzQ1Njc4OTAifQ.8XxPN0yJufkzy8TdEspyV-GqR1b1MF8aW_YVERdoRic"
)

// webApp mirrors the production fiber.Config JSON wiring (json/v2 encode +
// duplicate-key-tolerant decode) so c.Bind().Body and c.JSON behave in tests
// exactly as they do under app.New().
func webApp() *fiber.App {
	app := fiber.New(fiber.Config{
		JSONEncoder: func(v any) ([]byte, error) { return json.Marshal(v) },
		JSONDecoder: func(data []byte, v any) error {
			return json.Unmarshal(data, v, jsontext.AllowDuplicateNames(true))
		},
	})
	RegisterWeb(app, testSecret)
	return app
}

func do(t *testing.T, app *fiber.App, req *http.Request) (*http.Response, string) {
	t.Helper()
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 30 * time.Second, FailOnTimeout: true})
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	_ = resp.Body.Close()
	return resp, string(b)
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
	resp, body := do(t, webApp(), httptest.NewRequest(http.MethodGet, "/html", nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type = %q, want text/html", ct)
	}
	for _, want := range []string{"Hello, Alice", "apple", "banana", "cherry", "Total: 42"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
}

func TestCompute(t *testing.T) {
	app := webApp()
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
			resp, body := do(t, app, httptest.NewRequest(http.MethodGet, "/compute?"+tc.query, nil))
			if resp.StatusCode != tc.status {
				t.Fatalf("status = %d, want %d (body %s)", resp.StatusCode, tc.status, body)
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
	app := webApp()

	validReq := httptest.NewRequest(http.MethodPost, "/validate", strings.NewReader(valid))
	validReq.Header.Set("Content-Type", "application/json")
	resp, body := do(t, app, validReq)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("valid: status = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"valid":true`) {
		t.Fatalf("valid: body = %s, want valid:true", body)
	}

	invalidReq := httptest.NewRequest(http.MethodPost, "/validate", strings.NewReader(invalid))
	invalidReq.Header.Set("Content-Type", "application/json")
	resp, body = do(t, app, invalidReq)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid: status = %d, want 400 (body %s)", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"validation failed"`) {
		t.Fatalf("invalid: body = %s, want validation failed", body)
	}
}

func TestJWTSignThenVerify(t *testing.T) {
	app := webApp()

	resp, body := do(t, app, httptest.NewRequest(http.MethodGet, "/jwt/sign", nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sign: status = %d, want 200", resp.StatusCode)
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
	resp, body = do(t, app, req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("verify: status = %d, want 200 (body %s)", resp.StatusCode, body)
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
	app := webApp()
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
			resp, body := do(t, app, req)
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401 (body %s)", resp.StatusCode, body)
			}
			if !strings.Contains(body, `"invalid token"`) {
				t.Fatalf("body missing invalid token: %s", body)
			}
		})
	}
}
