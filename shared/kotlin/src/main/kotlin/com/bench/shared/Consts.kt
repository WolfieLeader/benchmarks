package com.bench.shared

/**
 * Cross-server constants: canonical error strings, request limits, and web-suite
 * canon. The error strings mirror `shared/go/consts` and `shared/rust/consts`
 * byte-for-byte so every framework emits identical error bodies — the contract
 * asserts them exactly.
 */
object Consts {
    // Error `error` field strings (the `{"error": ...}` value).
    const val ERR_INVALID_JSON = "invalid JSON body"
    const val ERR_REQUEST_TOO_LARGE = "request body too large"
    const val ERR_INVALID_FORM = "invalid form data"
    const val ERR_EXPECTED_FORM_CONTENT_TYPE =
        "expected content-type: application/x-www-form-urlencoded or multipart/form-data"
    const val ERR_INVALID_MULTIPART = "invalid multipart form data"
    const val ERR_EXPECTED_MULTIPART_CONTENT_TYPE = "expected content-type: multipart/form-data"
    const val ERR_FILE_NOT_FOUND = "file not found in form data"
    const val ERR_FILE_SIZE_EXCEEDED = "file size exceeds limit"
    const val ERR_INVALID_FILE_TYPE = "only text/plain files are allowed"
    const val ERR_NOT_PLAIN_TEXT = "file does not look like plain text"
    const val ERR_NOT_FOUND = "not found"
    const val ERR_INTERNAL = "internal error"

    // Web suite (Phase 3 contract) error strings.
    const val ERR_INVALID_TOKEN = "invalid token"
    const val ERR_VALIDATION_FAILED = "validation failed"
    const val ERR_INVALID_N = "invalid n"

    /** Global request-body cap (10 MiB) applied to every route. */
    const val MAX_REQUEST_BYTES: Long = 10L shl 20

    /** Per-file upload cap (1 MiB) enforced on `POST /params/file`. */
    const val MAX_FILE_BYTES: Long = 1L shl 20

    /** Bytes inspected when sniffing an upload's content type. */
    const val SNIFF_LEN: Int = 512

    /** Default `limit` when the `/params/search` query param is missing/unparseable. */
    const val DEFAULT_LIMIT: Long = 10

    // Web-suite canon (mirrors the contract's web.json + the Go conformance runner).

    /** `/compute` SHA-256 chain seed bytes. */
    const val COMPUTE_SEED = "benchmark"

    /** `/compute` clamp cap — n above this is rounded down, not rejected. */
    const val COMPUTE_MAX_ROUNDS: Long = 1_000_000

    /** `/jwt/sign` TTL: 1 hour (`exp = iat + 3600`), contract canon. */
    const val JWT_TTL_SECONDS: Long = 3600

    // `/jwt/sign` fixed claims (the dynamic iat/exp are added at sign time).
    const val JWT_SUBJECT = "1234567890"
    const val JWT_NAME = "John Doe"
    const val JWT_ADMIN = true
}
