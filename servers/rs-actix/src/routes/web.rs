//! Web-suite handlers: `GET /html`, `GET /jwt/sign`, `GET /jwt/verify`,
//! `POST /validate`, `GET /compute`. The canon (validation schema, JWT claims,
//! SHA chain, HTML) lives in `shared::web`; these handlers are the actix wiring
//! and stay byte-identical to rs-axum on the wire.
//!
//! The JWT secret is shared as `web::Data<String>` (an `Arc`), built once in
//! `main` and extracted by the two JWT handlers (guide rule 40).

use actix_web::http::header::{AUTHORIZATION, ContentType};
use actix_web::web::Bytes;
use actix_web::{HttpRequest, HttpResponse, web};
use serde_json::json;
use shared::Validate;
use shared::web::{self as shared_web, ValidatePayload};

use crate::error::ApiError;

/// Register the web routes (mounted at the app root, not under a scope).
pub fn config(cfg: &mut web::ServiceConfig) {
    cfg.route("/html", web::get().to(html))
        .route("/jwt/sign", web::get().to(jwt_sign))
        .route("/jwt/verify", web::get().to(jwt_verify))
        .route("/validate", web::post().to(validate))
        .route("/compute", web::get().to(compute));
}

async fn html() -> HttpResponse {
    HttpResponse::Ok()
        .content_type(ContentType::html())
        .body(shared_web::render_html())
}

async fn jwt_sign(secret: web::Data<String>) -> Result<HttpResponse, ApiError> {
    let token = shared_web::sign_jwt(&secret).map_err(|_| ApiError::Internal)?;
    Ok(HttpResponse::Ok().json(json!({ "token": token })))
}

async fn jwt_verify(req: HttpRequest, secret: web::Data<String>) -> Result<HttpResponse, ApiError> {
    let token = req
        .headers()
        .get(AUTHORIZATION)
        .and_then(|value| value.to_str().ok())
        .and_then(|value| value.strip_prefix("Bearer "))
        .map(str::trim)
        .filter(|token| !token.is_empty())
        .ok_or(ApiError::InvalidToken)?;
    let claims = shared_web::verify_jwt(token, &secret).ok_or(ApiError::InvalidToken)?;
    Ok(HttpResponse::Ok().json(claims))
}

async fn validate(body: Bytes) -> Result<HttpResponse, ApiError> {
    // Decode failure and validation failure are distinct 400s (mirror Go's split):
    // a malformed body is "invalid JSON body", a schema violation is "validation
    // failed".
    let payload: ValidatePayload =
        serde_json::from_slice(&body).map_err(|e| ApiError::InvalidJson(e.to_string()))?;
    payload
        .validate()
        .map_err(|e| ApiError::ValidationFailed(e.to_string()))?;
    Ok(HttpResponse::Ok().json(json!({ "valid": true })))
}

async fn compute(req: HttpRequest) -> Result<HttpResponse, ApiError> {
    let rounds = parse_rounds(req.query_string()).ok_or(ApiError::InvalidN)?;
    Ok(HttpResponse::Ok().json(json!({ "result": shared_web::compute_chain(rounds) })))
}

/// Parse the `n` query param, requiring an integer `>= 1` and clamping to the
/// canon cap. `None` (missing, non-numeric, zero, or negative) is a 400.
/// No trimming: `u64::from_str` matches Go's `strconv.Atoi` in rejecting
/// whitespace, underscores (`1_000`), and signs other than a leading `+`.
fn parse_rounds(query: &str) -> Option<u64> {
    let n: u64 = form_urlencoded::parse(query.as_bytes())
        .find(|(key, _)| key == "n")
        .and_then(|(_, value)| value.parse().ok())?;
    (n >= 1).then_some(n.min(shared_web::COMPUTE_MAX_ROUNDS))
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
