from typing import TypedDict


INVALID_JSON_BODY = "invalid JSON body"
INVALID_FORM_DATA = "invalid form data"
EXPECTED_FORM_CONTENT_TYPE = "expected content-type: application/x-www-form-urlencoded or multipart/form-data"
INVALID_MULTIPART = "invalid multipart form data"
EXPECTED_MULTIPART_CONTENT_TYPE = "expected content-type: multipart/form-data"
FILE_NOT_FOUND = "file not found in form data"
NOT_FOUND = "not found"
FILE_SIZE_EXCEEDS = "file size exceeds limit"
ONLY_TEXT_PLAIN = "only text/plain files are allowed"
FILE_NOT_TEXT = "file does not look like plain text"
INTERNAL_ERROR = "internal error"
REQUEST_TOO_LARGE = "request body too large"

# Web suite (PLAN §3) — house "invalid <thing>" / "<what> failed" style, asserted
# by the /validate, /jwt/verify and /compute referees (contract/web.json). Promoted
# from py-flask's in-server consts once fastapi/django gained web: true (3 consumers).
VALIDATION_FAILED = "validation failed"
# S105: the /jwt/verify error-message string (contract canon), not a credential —
# the name just happens to contain "TOKEN".
INVALID_TOKEN = "invalid token"  # noqa: S105
INVALID_N = "invalid n"


class ErrorResponse(TypedDict, total=False):
    error: str
    details: str


def make_error(error: str, detail: str | Exception | None = None) -> ErrorResponse:
    msg = str(detail) if detail is not None else None
    if msg:
        return {"error": error, "details": msg}
    return {"error": error}
