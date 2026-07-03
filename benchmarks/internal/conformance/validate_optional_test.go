package conformance

import (
	"encoding/json"
	"testing"
)

// TestOptionalMatcher pins the $optional semantics: the key MAY be absent; if
// present, any non-null value passes; present-but-null fails; and $optional must
// compose with the strict extra-keys check.
func TestOptionalMatcher(t *testing.T) {
	want := mustJSON(t, `{"error": "boom", "details": "$optional"}`)

	cases := []struct {
		name    string
		got     string
		strict  bool
		wantErr bool
	}{
		{"absent passes", `{"error": "boom"}`, true, false},
		{"present string passes", `{"error": "boom", "details": "why"}`, true, false},
		{"present number passes", `{"error": "boom", "details": 42}`, true, false},
		{"present null fails", `{"error": "boom", "details": null}`, true, true},
		{"absent with unexpected extra key fails (strict)", `{"error": "boom", "extra": 1}`, true, true},
		{"present optional is never an unexpected key", `{"error": "boom", "details": "x"}`, true, false},
		{"wrong literal for required key fails", `{"error": "nope"}`, true, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := matchJSON(want, mustJSON(t, tc.got), tc.strict)
			if (err != nil) != tc.wantErr {
				t.Fatalf("matchJSON err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

// TestOptionalAsScalarValueRejected guards the misuse: $optional is only valid as
// an object key, never as a standalone value.
func TestOptionalAsScalarValueRejected(t *testing.T) {
	if err := matchJSON("$optional", "anything", true); err == nil {
		t.Fatal("expected error when $optional is used as a scalar value")
	}
}

func mustJSON(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("bad test JSON %q: %v", s, err)
	}
	return v
}
