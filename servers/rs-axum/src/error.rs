//! The one place that decides HTTP status + body for every error arm (guide
//! rule 10). Handlers return `Result<_, ApiError>` and stay `?`-clean; this
//! `IntoResponse` renders the uniform `{"error": string, "details"?: string}`
//! shape. A `shared::DbError` collapses to a generic 500 so no driver string ever
//! leaks into a response (guide rule 11).

use axum::Json;
use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use serde::Serialize;
use shared::DbError;
use shared::consts;

#[derive(Debug)]
pub enum ApiError {
    /// 404 — carries the `details` string ($present in the contract).
    NotFound(String),
    /// 400 — malformed JSON or a failed validation; `details` is the cause.
    InvalidJson(String),
    /// 400 — `/validate` body decoded but failed the schema rules.
    ValidationFailed(String),
    /// 400 — `/compute` reached without a valid integer `n >= 1`.
    InvalidN,
    /// 401 — `/jwt/verify` token missing, malformed, wrong-signature, or expired.
    InvalidToken,
    /// 400 — form route reached with a non-form content type.
    InvalidForm,
    /// 400 — file route reached with a non-multipart content type.
    InvalidMultipartContentType,
    /// 400 — multipart parsed but carried no `file` field.
    FileNotFound(String),
    /// 415 — upload is not (declared or sniffed as) `text/plain`.
    InvalidFileType,
    /// 415 — upload declared text but its bytes are not plain text.
    NotPlainText,
    /// 413 — upload exceeds the 1 MiB file cap.
    FileTooLarge,
    /// 500 — an internal failure (DB error); details are never exposed.
    Internal,
}

#[derive(Serialize)]
struct ErrorBody {
    error: &'static str,
    #[serde(skip_serializing_if = "Option::is_none")]
    details: Option<String>,
}

impl IntoResponse for ApiError {
    fn into_response(self) -> Response {
        let (status, error, details) = match self {
            Self::NotFound(details) => {
                (StatusCode::NOT_FOUND, consts::ERR_NOT_FOUND, Some(details))
            }
            Self::InvalidJson(details) => (
                StatusCode::BAD_REQUEST,
                consts::ERR_INVALID_JSON,
                Some(details),
            ),
            Self::ValidationFailed(details) => (
                StatusCode::BAD_REQUEST,
                consts::ERR_VALIDATION_FAILED,
                Some(details),
            ),
            Self::InvalidN => (
                StatusCode::BAD_REQUEST,
                consts::ERR_INVALID_N,
                Some("n must be an integer >= 1".to_string()),
            ),
            Self::InvalidToken => (StatusCode::UNAUTHORIZED, consts::ERR_INVALID_TOKEN, None),
            Self::InvalidForm => (
                StatusCode::BAD_REQUEST,
                consts::ERR_INVALID_FORM,
                Some(consts::ERR_EXPECTED_FORM_CONTENT_TYPE.to_string()),
            ),
            Self::InvalidMultipartContentType => (
                StatusCode::BAD_REQUEST,
                consts::ERR_INVALID_MULTIPART,
                Some(consts::ERR_EXPECTED_MULTIPART_CONTENT_TYPE.to_string()),
            ),
            Self::FileNotFound(details) => (
                StatusCode::BAD_REQUEST,
                consts::ERR_FILE_NOT_FOUND,
                Some(details),
            ),
            Self::InvalidFileType => (
                StatusCode::UNSUPPORTED_MEDIA_TYPE,
                consts::ERR_INVALID_FILE_TYPE,
                None,
            ),
            Self::NotPlainText => (
                StatusCode::UNSUPPORTED_MEDIA_TYPE,
                consts::ERR_NOT_PLAIN_TEXT,
                None,
            ),
            Self::FileTooLarge => (
                StatusCode::PAYLOAD_TOO_LARGE,
                consts::ERR_FILE_SIZE_EXCEEDED,
                None,
            ),
            Self::Internal => (
                StatusCode::INTERNAL_SERVER_ERROR,
                consts::ERR_INTERNAL,
                None,
            ),
        };
        (status, Json(ErrorBody { error, details })).into_response()
    }
}

impl From<DbError> for ApiError {
    fn from(_: DbError) -> Self {
        // Driver error strings are internal; the contract asserts strict bodies.
        Self::Internal
    }
}
