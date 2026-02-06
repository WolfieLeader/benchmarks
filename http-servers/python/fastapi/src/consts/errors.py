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


class ErrorResponse(TypedDict, total=False):
    error: str
    details: str


def make_error(error: str, detail: str | Exception | None = None) -> ErrorResponse:
    if isinstance(detail, Exception):
        msg = str(detail)
        return {"error": error, "details": msg} if msg else {"error": error}
    if detail:
        return {"error": error, "details": detail}
    return {"error": error}
