//! Web-suite infrastructure shared by every server that implements it (PLAN §5):
//! the `/validate` request schema (validator rules), the canon constants for
//! `/compute` and `/jwt/sign`, the HS256 sign/verify helpers (house pick:
//! `jsonwebtoken`), and the SHA-256 compute chain. Only framework-independent
//! contract values live here — the handlers stay per-framework and idiomatic
//! (PLAN §3, the idiom boundary).

use jsonwebtoken::{Algorithm, DecodingKey, EncodingKey, Header, Validation, decode, encode};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use validator::{Validate, ValidationError};

// --- Compute canon -----------------------------------------------------------

/// `GET /compute` applies SHA-256 to `COMPUTE_SEED` n times and returns the
/// lowercase-hex digest. The seed must equal the conformance runner's
/// `$sha256chain` seed (benchmark/internal/conformance).
pub const COMPUTE_SEED: &str = "benchmark";
/// n is clamped to this cap (bounds the per-request CPU work); above it the
/// response is still 200 and equals the cap-rounds chain.
pub const COMPUTE_MAX_ROUNDS: u64 = 1_000_000;

// --- JWT canon ---------------------------------------------------------------

/// `GET /jwt/sign` issues an HS256 token with these fixed claims plus a dynamic
/// `iat` and `exp` (= iat + `JWT_TTL_SECS`), signed with the shared `JWT_SECRET`.
pub const JWT_SUBJECT: &str = "1234567890";
pub const JWT_NAME: &str = "John Doe";
pub const JWT_ADMIN: bool = true;
/// Canon TTL = 1 hour (`exp = iat + 3600`).
pub const JWT_TTL_SECS: u64 = 3600;

/// The five canon claims carried by a `/jwt/sign` token and echoed by
/// `/jwt/verify`. Serializes to exactly `{sub, name, admin, iat, exp}`.
#[derive(Debug, Serialize, Deserialize)]
pub struct Claims {
    pub sub: String,
    pub name: String,
    pub admin: bool,
    pub iat: u64,
    pub exp: u64,
}

/// Sign the canon claims as an HS256 token with a fresh `iat`/`exp`.
pub fn sign_jwt(secret: &str) -> Result<String, jsonwebtoken::errors::Error> {
    let iat = jsonwebtoken::get_current_timestamp();
    let claims = Claims {
        sub: JWT_SUBJECT.to_string(),
        name: JWT_NAME.to_string(),
        admin: JWT_ADMIN,
        iat,
        exp: iat + JWT_TTL_SECS,
    };
    encode(
        &Header::new(Algorithm::HS256),
        &claims,
        &EncodingKey::from_secret(secret.as_bytes()),
    )
}

/// Verify an HS256 token against `secret`, returning the claims only when the
/// signature checks out **and** `exp` is present and unexpired. The algorithm is
/// pinned to HS256 (a token that lies about its `alg` cannot slip through) and
/// `exp` is both required and validated (a signature-valid but expired token is
/// rejected). Any failure collapses to `None` — the caller maps that to a 401.
pub fn verify_jwt(token: &str, secret: &str) -> Option<Claims> {
    let mut validation = Validation::new(Algorithm::HS256);
    validation.validate_exp = true;
    validation.set_required_spec_claims(&["exp"]);
    decode::<Claims>(
        token,
        &DecodingKey::from_secret(secret.as_bytes()),
        &validation,
    )
    .map(|data| data.claims)
    .ok()
}

// --- Compute chain -----------------------------------------------------------

/// Apply SHA-256 to `COMPUTE_SEED` `rounds` times, returning the lowercase-hex
/// digest. Callers clamp `rounds` to `COMPUTE_MAX_ROUNDS` first.
pub fn compute_chain(rounds: u64) -> String {
    let mut state = COMPUTE_SEED.as_bytes().to_vec();
    for _ in 0..rounds {
        state = Sha256::digest(&state).to_vec();
    }
    hex::encode(state)
}

// --- HTML canon --------------------------------------------------------------

/// Render the `/html` canon page: a greeting, a fruit list, and a labeled total.
/// The values are fixed cross-framework canon, so a `format!` template is the
/// honest minimal render — no engine or untrusted interpolation is involved.
pub fn render_html() -> String {
    use std::fmt::Write as _;

    let name = "Alice";
    let fruits = ["apple", "banana", "cherry"];
    let total = 42;
    let mut items = String::new();
    for fruit in fruits {
        // Writing to a String is infallible, so the Result is discarded.
        let _ = writeln!(items, "    <li>{fruit}</li>");
    }
    format!(
        "<!DOCTYPE html>\n<html lang=\"en\">\n<head><meta charset=\"utf-8\"><title>Benchmark</title></head>\n<body>\n  <h1>Hello, {name}</h1>\n  <ul>\n{items}  </ul>\n  <p>Total: {total}</p>\n</body>\n</html>\n"
    )
}

// --- /validate schema (~4 levels) --------------------------------------------

/// `POST /validate` request schema. Mirrors the cross-language canon (the Go
/// `web.ValidatePayload`, the TS zod schema, the pydantic model): a valid object
/// is `200 {"valid":true}`, any violation is `400 {"error":"validation failed"}`.
#[derive(Debug, Deserialize, Validate)]
pub struct ValidatePayload {
    #[validate(required, nested)]
    pub user: Option<ValidateUser>,
    #[validate(length(min = 1), nested)]
    pub items: Vec<ValidateItem>,
    #[validate(range(min = 0.0))]
    pub total: f64,
}

/// A UUID id, an email, and a required profile. `Serialize` is derived because
/// validator embeds the field value in its error report (`add_param`), which
/// requires every validated nested type to be serializable.
#[derive(Debug, Deserialize, Serialize, Validate)]
pub struct ValidateUser {
    #[validate(custom(function = "validate_uuid"))]
    pub id: String,
    #[validate(email)]
    pub email: String,
    #[validate(required, nested)]
    pub profile: Option<ValidateProfile>,
}

/// An age range, a role enum, and nested preferences. `age` is not required:
/// when omitted it defaults to 0 (mirroring Go's zero-value decode), which is
/// in range — an absent age validates.
#[derive(Debug, Deserialize, Serialize, Validate)]
pub struct ValidateProfile {
    #[serde(default)]
    #[validate(range(min = 0, max = 120))]
    pub age: i64,
    #[validate(custom(function = "validate_role"))]
    pub role: String,
    #[validate(required, nested)]
    pub preferences: Option<ValidatePreferences>,
}

/// The deepest level: a theme enum and a notifications flag.
#[derive(Debug, Deserialize, Serialize, Validate)]
pub struct ValidatePreferences {
    #[validate(custom(function = "validate_theme"))]
    pub theme: String,
    #[validate(required)]
    pub notifications: Option<bool>,
}

/// One line item: a sku, an in-range quantity, and a tags list (tags may be
/// empty, so it carries no presence rule).
#[derive(Debug, Deserialize, Serialize, Validate)]
pub struct ValidateItem {
    #[validate(length(min = 1))]
    pub sku: String,
    #[validate(range(min = 1, max = 100))]
    pub quantity: i64,
    pub tags: Vec<String>,
}

fn validate_uuid(value: &str) -> Result<(), ValidationError> {
    uuid::Uuid::parse_str(value)
        .map(|_| ())
        .map_err(|_| ValidationError::new("uuid"))
}

fn validate_role(value: &str) -> Result<(), ValidationError> {
    if matches!(value, "admin" | "user" | "guest") {
        Ok(())
    } else {
        Err(ValidationError::new("role"))
    }
}

fn validate_theme(value: &str) -> Result<(), ValidationError> {
    if matches!(value, "light" | "dark") {
        Ok(())
    } else {
        Err(ValidationError::new("theme"))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // The dev-default shared secret (matches env.rs and the contract harness).
    const SECRET: &str = "benchmarks-shared-jwt-secret-dev-default";
    // Static tokens from contract/web.json. BAD_SIG is signed with a throwaway
    // secret; EXPIRED is signed with the dev-default secret but expired in 2020.
    const BAD_SIG: &str = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhZG1pbiI6dHJ1ZSwiZXhwIjo0MTAyNDQ0ODAwLCJpYXQiOjE3MzU2ODk2MDAsIm5hbWUiOiJKb2huIERvZSIsInN1YiI6IjEyMzQ1Njc4OTAifQ.J75FiSXpAhQxN9jiUjBHADeu_su1WJnZjJqDXI4aOWw";
    const EXPIRED: &str = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhZG1pbiI6dHJ1ZSwiZXhwIjoxNTc3ODQwNDAwLCJpYXQiOjE1Nzc4MzY4MDAsIm5hbWUiOiJKb2huIERvZSIsInN1YiI6IjEyMzQ1Njc4OTAifQ.8XxPN0yJufkzy8TdEspyV-GqR1b1MF8aW_YVERdoRic";

    const VALID_BODY: &str = r#"{
        "user": {
            "id": "3f1a2b3c-4d5e-6f70-8192-a3b4c5d6e7f8",
            "email": "alice@conformance-suite.com",
            "profile": {
                "age": 30,
                "role": "admin",
                "preferences": { "theme": "dark", "notifications": true }
            }
        },
        "items": [
            { "sku": "SKU-1", "quantity": 2, "tags": ["new", "featured"] },
            { "sku": "SKU-2", "quantity": 100, "tags": [] }
        ],
        "total": 42.5
    }"#;

    const INVALID_BODY: &str = r#"{
        "user": {
            "id": "not-a-uuid",
            "email": "not-an-email",
            "profile": {
                "age": 200,
                "role": "superuser",
                "preferences": { "theme": "neon", "notifications": true }
            }
        },
        "items": [{ "sku": "SKU-1", "quantity": 0, "tags": ["x"] }],
        "total": -5
    }"#;

    #[test]
    fn sign_then_verify_roundtrips() {
        let token = sign_jwt(SECRET).expect("sign");
        let claims = verify_jwt(&token, SECRET).expect("freshly signed token verifies");
        assert_eq!(claims.sub, JWT_SUBJECT);
        assert_eq!(claims.name, JWT_NAME);
        assert!(claims.admin);
        assert_eq!(claims.exp, claims.iat + JWT_TTL_SECS);
    }

    #[test]
    fn rejects_bad_signature() {
        assert!(verify_jwt(BAD_SIG, SECRET).is_none());
    }

    #[test]
    fn rejects_expired_token() {
        assert!(verify_jwt(EXPIRED, SECRET).is_none());
    }

    #[test]
    fn rejects_malformed_token() {
        assert!(verify_jwt("not-a-jwt", SECRET).is_none());
    }

    #[test]
    fn rejects_wrong_secret() {
        let token = sign_jwt(SECRET).expect("sign");
        assert!(verify_jwt(&token, "a-different-secret").is_none());
    }

    #[test]
    fn compute_chain_known_vectors() {
        // SHA-256 applied n times to the seed bytes "benchmark", lowercase hex.
        assert_eq!(
            compute_chain(1),
            "0e89820860c342f2c7ec694d144023b10301c2accdd078cb5167a06d0c3d5bcc"
        );
        assert_eq!(
            compute_chain(2),
            "b061accddb3b47684b3bd36291c9b219cfbd43bb074f72251f6574b073425003"
        );
    }

    #[test]
    fn validate_accepts_valid_object() {
        let payload: ValidatePayload = serde_json::from_str(VALID_BODY).expect("decode");
        assert!(payload.validate().is_ok());
    }

    #[test]
    fn validate_rejects_invalid_object() {
        let payload: ValidatePayload = serde_json::from_str(INVALID_BODY).expect("decode");
        assert!(payload.validate().is_err());
    }

    /// Decode `VALID_BODY`, apply `patch` to the parsed JSON, then decode+validate.
    /// `Err(true)` = decode failed; `Err(false)` = validation failed; `Ok(())` = valid.
    fn decode_and_validate(patch: impl FnOnce(&mut serde_json::Value)) -> Result<(), bool> {
        let mut body: serde_json::Value = serde_json::from_str(VALID_BODY).expect("base body");
        patch(&mut body);
        let payload: ValidatePayload = serde_json::from_value(body).map_err(|_| true)?;
        payload.validate().map_err(|_| false)
    }

    #[test]
    fn validate_accepts_omitted_age() {
        // age is not required (canon): omitted → defaults to 0, which is in range.
        let result = decode_and_validate(|body| {
            body["user"]["profile"]
                .as_object_mut()
                .expect("profile object")
                .remove("age");
        });
        assert_eq!(result, Ok(()));
    }

    #[test]
    fn validate_rejects_empty_items() {
        let result = decode_and_validate(|body| body["items"] = serde_json::json!([]));
        assert_eq!(result, Err(false), "empty items must fail validation");
    }

    #[test]
    fn validate_rejects_empty_sku() {
        let result = decode_and_validate(|body| body["items"][0]["sku"] = "".into());
        assert_eq!(result, Err(false), "empty sku must fail validation");
    }
}
