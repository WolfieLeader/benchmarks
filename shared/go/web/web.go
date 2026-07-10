// Package web holds the web-suite infrastructure shared across every server that
// implements it (PLAN §5): the /validate request schema (validator/v10 rules) and
// the canon constants for /compute and /jwt/sign. Only framework-independent
// contract values live here — the handlers themselves stay per-framework and
// idiomatic (PLAN §3, the idiom boundary).
package web

import "time"

// Compute canon: GET /compute applies SHA-256 to ComputeSeed n times and returns
// the lowercase-hex digest. n must be an integer in [1, ComputeMaxRounds]; above
// the cap it is clamped (bounds the per-request CPU work). The seed must equal
// the conformance runner's $sha256chain seed (benchmark/internal/conformance).
const (
	ComputeSeed      = "benchmark"
	ComputeMaxRounds = 1_000_000
)

// JWT canon: GET /jwt/sign issues an HS256 token with these fixed claims plus a
// dynamic iat and exp (= iat + JWTTTL), signed with the shared JWT_SECRET.
const (
	JWTSubject = "1234567890"
	JWTName    = "John Doe"
	JWTAdmin   = true
	JWTTTL     = time.Hour
)

// ValidatePayload is the POST /validate request schema (~4 levels deep). Servers
// bind the JSON body into it and run validator.Struct: a fully valid object is
// 200 {"valid":true}, any violation is 400 {"error":"validation failed", details}.
type ValidatePayload struct {
	User  *ValidateUser  `json:"user"  validate:"required"`
	Items []ValidateItem `json:"items" validate:"required,min=1,dive"`
	Total float64        `json:"total" validate:"gte=0"`
}

// ValidateUser is the user object: a UUID id, an email, and a required profile.
type ValidateUser struct {
	ID      string           `json:"id"      validate:"required,uuid"`
	Email   string           `json:"email"   validate:"required,email"`
	Profile *ValidateProfile `json:"profile" validate:"required"`
}

// ValidateProfile carries an age range, a role enum, and nested preferences.
type ValidateProfile struct {
	Age         int                  `json:"age"         validate:"gte=0,lte=120"`
	Role        string               `json:"role"        validate:"required,oneof=admin user guest"`
	Preferences *ValidatePreferences `json:"preferences" validate:"required"`
}

// ValidatePreferences is the deepest level: a theme enum and a notifications flag.
type ValidatePreferences struct {
	Theme         string `json:"theme"         validate:"required,oneof=light dark"`
	Notifications *bool  `json:"notifications" validate:"required"`
}

// ValidateItem is one line item: a sku, an in-range quantity, and a tags list
// (tags may be empty, so it carries no presence rule).
type ValidateItem struct {
	SKU      string   `json:"sku"      validate:"required"`
	Quantity int      `json:"quantity" validate:"gte=1,lte=100"`
	Tags     []string `json:"tags"`
}
