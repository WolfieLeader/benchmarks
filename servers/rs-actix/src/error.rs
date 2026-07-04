//! The one place that decides HTTP status + body for every error arm (guide
//! rule 10). Handlers return `Result<_, ApiError>` and stay `?`-clean; actix's
//! `ResponseError` renders the uniform `{"error": string, "details"?: string}`
//! shape. A `shared::DbError` collapses to a generic 500 so no driver string ever
//! leaks into a response (guide rule 11).

use actix_web::http::StatusCode;
use actix_web::http::header::ContentType;
use actix_web::{HttpResponse, ResponseError};
use serde::Serialize;
use shared::DbError;
use shared::consts;

#[derive(Debug)]
pub enum ApiError {
    /// 404 — carries the `details` string ($present in the contract).
    NotFound(String),
    /// 400 — malformed JSON or a failed validation; `details` is the cause.
    InvalidJson(String),
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

impl ApiError {
    /// The `error` field string and (optional) `details` for this variant.
    fn parts(&self) -> (&'static str, Option<String>) {
        match self {
            Self::NotFound(details) => (consts::ERR_NOT_FOUND, Some(details.clone())),
            Self::InvalidJson(details) => (consts::ERR_INVALID_JSON, Some(details.clone())),
            Self::InvalidForm => (
                consts::ERR_INVALID_FORM,
                Some(consts::ERR_EXPECTED_FORM_CONTENT_TYPE.to_string()),
            ),
            Self::InvalidMultipartContentType => (
                consts::ERR_INVALID_MULTIPART,
                Some(consts::ERR_EXPECTED_MULTIPART_CONTENT_TYPE.to_string()),
            ),
            Self::FileNotFound(details) => (consts::ERR_FILE_NOT_FOUND, Some(details.clone())),
            Self::InvalidFileType => (consts::ERR_INVALID_FILE_TYPE, None),
            Self::NotPlainText => (consts::ERR_NOT_PLAIN_TEXT, None),
            Self::FileTooLarge => (consts::ERR_FILE_SIZE_EXCEEDED, None),
            Self::Internal => (consts::ERR_INTERNAL, None),
        }
    }
}

// `Display` is required by `ResponseError`; it feeds actix's dev logging only —
// the wire body comes from `error_response`, not this text.
impl std::fmt::Display for ApiError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        let (error, _) = self.parts();
        f.write_str(error)
    }
}

impl ResponseError for ApiError {
    fn status_code(&self) -> StatusCode {
        match self {
            Self::NotFound(_) => StatusCode::NOT_FOUND,
            Self::InvalidJson(_)
            | Self::InvalidForm
            | Self::InvalidMultipartContentType
            | Self::FileNotFound(_) => StatusCode::BAD_REQUEST,
            Self::InvalidFileType | Self::NotPlainText => StatusCode::UNSUPPORTED_MEDIA_TYPE,
            Self::FileTooLarge => StatusCode::PAYLOAD_TOO_LARGE,
            Self::Internal => StatusCode::INTERNAL_SERVER_ERROR,
        }
    }

    fn error_response(&self) -> HttpResponse {
        let (error, details) = self.parts();
        HttpResponse::build(self.status_code())
            .insert_header(ContentType::json())
            .json(ErrorBody { error, details })
    }
}

impl From<DbError> for ApiError {
    fn from(_: DbError) -> Self {
        // Driver error strings are internal; the contract asserts strict bodies.
        Self::Internal
    }
}
