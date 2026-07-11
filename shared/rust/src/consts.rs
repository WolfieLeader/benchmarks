//! Cross-server constants: canonical error strings and request limits.
//!
//! These mirror `shared/go/consts` (and the TS `@bench/shared` consts) verbatim
//! so every framework emits byte-identical error bodies — the contract asserts
//! them exactly.

// Error `error` field strings (the `{"error": ...}` value).
pub const ERR_INVALID_JSON: &str = "invalid JSON body";
pub const ERR_REQUEST_TOO_LARGE: &str = "request body too large";
pub const ERR_INVALID_FORM: &str = "invalid form data";
pub const ERR_EXPECTED_FORM_CONTENT_TYPE: &str =
    "expected content-type: application/x-www-form-urlencoded or multipart/form-data";
pub const ERR_INVALID_MULTIPART: &str = "invalid multipart form data";
pub const ERR_EXPECTED_MULTIPART_CONTENT_TYPE: &str = "expected content-type: multipart/form-data";
pub const ERR_FILE_NOT_FOUND: &str = "file not found in form data";
pub const ERR_FILE_SIZE_EXCEEDED: &str = "file size exceeds limit";
pub const ERR_INVALID_FILE_TYPE: &str = "only text/plain files are allowed";
pub const ERR_NOT_PLAIN_TEXT: &str = "file does not look like plain text";
pub const ERR_NOT_FOUND: &str = "not found";
pub const ERR_INTERNAL: &str = "internal error";

// Web suite (PLAN §5) — house "invalid <thing>" / "<what> failed" style.
pub const ERR_INVALID_TOKEN: &str = "invalid token";
pub const ERR_VALIDATION_FAILED: &str = "validation failed";
pub const ERR_INVALID_N: &str = "invalid n";

/// Global request-body cap (10 MiB) applied to every route.
pub const MAX_REQUEST_BYTES: usize = 10 << 20;
/// Per-file upload cap (1 MiB) enforced on `POST /params/file`.
pub const MAX_FILE_BYTES: usize = 1 << 20;
/// Bytes inspected when sniffing an upload's content type.
pub const SNIFF_LEN: usize = 512;
/// Default `limit` when the query param is missing or unparseable.
pub const DEFAULT_LIMIT: i64 = 10;
