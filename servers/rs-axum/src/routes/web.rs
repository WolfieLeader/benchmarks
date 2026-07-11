//! Web-suite handlers: `GET /html`, `GET /jwt/sign`, `GET /jwt/verify`,
//! `POST /validate`, `GET /compute`. The canon (validation schema, JWT claims,
//! SHA chain, HTML) lives in `shared::web`; these handlers are the axum wiring.
//!
//! None of the five routes touch the DB, so they need only the JWT secret, not
//! the `AppState` repositories. The sub-router carries the secret as its own
//! `State`, then `.with_state(...)` resolves it so the router merges into the
//! main `Router<AppState>` without disturbing the DB handlers' state (guide
//! rule 35).

use std::sync::Arc;

use axum::extract::{RawQuery, State};
use axum::http::HeaderMap;
use axum::http::header::AUTHORIZATION;
use axum::response::Html;
use axum::{Json, Router, routing::get, routing::post};
use serde_json::{Value, json};
use shared::Validate;
use shared::web::{self, Claims, ValidatePayload};

use crate::error::ApiError;
use crate::state::AppState;

/// Build the web sub-router. `jwt_secret` is baked in as `State`, yielding a
/// router that merges into the main `Router<AppState>`.
pub fn router(jwt_secret: Arc<str>) -> Router<AppState> {
    Router::new()
        .route("/html", get(html))
        .route("/jwt/sign", get(jwt_sign))
        .route("/jwt/verify", get(jwt_verify))
        .route("/validate", post(validate))
        .route("/compute", get(compute))
        .with_state(jwt_secret)
}

async fn html() -> Html<String> {
    Html(web::render_html())
}

async fn jwt_sign(State(secret): State<Arc<str>>) -> Result<Json<Value>, ApiError> {
    let token = web::sign_jwt(&secret).map_err(|_| ApiError::Internal)?;
    Ok(Json(json!({ "token": token })))
}

async fn jwt_verify(
    State(secret): State<Arc<str>>,
    headers: HeaderMap,
) -> Result<Json<Claims>, ApiError> {
    let token = headers
        .get(AUTHORIZATION)
        .and_then(|value| value.to_str().ok())
        .and_then(|value| value.strip_prefix("Bearer "))
        .map(str::trim)
        .filter(|token| !token.is_empty())
        .ok_or(ApiError::InvalidToken)?;
    let claims = web::verify_jwt(token, &secret).ok_or(ApiError::InvalidToken)?;
    Ok(Json(claims))
}

async fn validate(body: axum::body::Bytes) -> Result<Json<Value>, ApiError> {
    // Decode failure and validation failure are distinct 400s (guide: mirror Go's
    // split): a malformed body is "invalid JSON body", a schema violation is
    // "validation failed".
    let payload: ValidatePayload =
        serde_json::from_slice(&body).map_err(|e| ApiError::InvalidJson(e.to_string()))?;
    payload
        .validate()
        .map_err(|e| ApiError::ValidationFailed(e.to_string()))?;
    Ok(Json(json!({ "valid": true })))
}

async fn compute(RawQuery(query): RawQuery) -> Result<Json<Value>, ApiError> {
    let rounds = query
        .as_deref()
        .and_then(parse_rounds)
        .ok_or(ApiError::InvalidN)?;
    Ok(Json(json!({ "result": web::compute_chain(rounds) })))
}

/// Parse the `n` query param, requiring an integer `>= 1` and clamping to the
/// canon cap. `None` (missing, non-numeric, zero, or negative) is a 400.
/// No trimming: `u64::from_str` matches Go's `strconv.Atoi` in rejecting
/// whitespace, underscores (`1_000`), and negative signs. (A literal `+` in
/// the query string is decoded to a space by `form_urlencoded` before parsing
/// — same as Go's `url.Query()` — so `n=+5` arrives as `" 5"` and is rejected.)
fn parse_rounds(query: &str) -> Option<u64> {
    let n: u64 = form_urlencoded::parse(query.as_bytes())
        .find(|(key, _)| key == "n")
        .and_then(|(_, value)| value.parse().ok())?;
    (n >= 1).then_some(n.min(web::COMPUTE_MAX_ROUNDS))
}

#[cfg(test)]
mod tests {
    use super::parse_rounds;

    #[test]
    fn parse_rounds_accepts_and_clamps() {
        assert_eq!(parse_rounds("n=1"), Some(1));
        assert_eq!(parse_rounds("n=1000"), Some(1000));
        // Above the cap: clamped, not rejected.
        assert_eq!(parse_rounds("n=5000000"), Some(1_000_000));
    }

    #[test]
    fn parse_rounds_rejects_invalid() {
        assert_eq!(parse_rounds(""), None); // missing
        assert_eq!(parse_rounds("n=abc"), None); // non-numeric
        assert_eq!(parse_rounds("n=0"), None); // zero
        assert_eq!(parse_rounds("n=-5"), None); // negative
        assert_eq!(parse_rounds("n=1_000"), None); // Rust literal syntax is not an integer
        assert_eq!(parse_rounds("n=1.5"), None); // fractional
        assert_eq!(parse_rounds("n="), None); // empty value
    }
}
