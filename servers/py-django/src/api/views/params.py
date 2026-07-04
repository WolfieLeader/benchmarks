from __future__ import annotations

import json
from typing import Any

from django.http import HttpRequest, HttpResponse, JsonResponse

from bench_shared.consts import DEFAULT_LIMIT, MAX_FILE_BYTES, NULL_BYTE, SAFE_INT_LIMIT, SNIFF_LEN
from bench_shared.errors import (
    EXPECTED_FORM_CONTENT_TYPE,
    EXPECTED_MULTIPART_CONTENT_TYPE,
    FILE_NOT_FOUND,
    FILE_NOT_TEXT,
    FILE_SIZE_EXCEEDS,
    INVALID_FORM_DATA,
    INVALID_JSON_BODY,
    INVALID_MULTIPART,
    ONLY_TEXT_PLAIN,
    make_error,
)


def _error(error: str, status: int, detail: str | Exception | None = None) -> JsonResponse:
    return JsonResponse(make_error(error, detail), status=status)


def _strip_or(value: str | None, default: str) -> str:
    stripped = value.strip() if value else ""
    return stripped or default


def _parse_safe_int(value: str | None, default: int) -> int:
    if value is None or "." in value:
        return default
    try:
        num = int(value)
    except ValueError:
        return default
    if -SAFE_INT_LIMIT <= num <= SAFE_INT_LIMIT:
        return num
    return default


async def search_params(request: HttpRequest) -> JsonResponse:
    return JsonResponse(
        {
            "search": _strip_or(request.GET.get("q"), "none"),
            "limit": _parse_safe_int(request.GET.get("limit"), DEFAULT_LIMIT),
        }
    )


async def url_params(request: HttpRequest, dynamic: str) -> JsonResponse:
    return JsonResponse({"dynamic": dynamic})


async def header_params(request: HttpRequest) -> JsonResponse:
    return JsonResponse({"header": _strip_or(request.headers.get("X-Custom-Header"), "none")})


async def body_params(request: HttpRequest) -> JsonResponse:
    try:
        payload: Any = json.loads(request.body)
    except ValueError as e:
        return _error(INVALID_JSON_BODY, 400, e)
    if not isinstance(payload, dict):
        return _error(INVALID_JSON_BODY, 400, "expected a JSON object")
    return JsonResponse({"body": payload})


async def cookie_params(request: HttpRequest) -> JsonResponse:
    response = JsonResponse({"cookie": _strip_or(request.COOKIES.get("foo"), "none")})
    response.set_cookie("bar", "12345", max_age=10, httponly=True, path="/")
    return response


async def form_params(request: HttpRequest) -> HttpResponse:
    content_type = request.headers.get("content-type", "").lower()
    if not (
        content_type.startswith("application/x-www-form-urlencoded") or content_type.startswith("multipart/form-data")
    ):
        return _error(INVALID_FORM_DATA, 400, EXPECTED_FORM_CONTENT_TYPE)

    name = _strip_or(request.POST.get("name"), "none")
    age_raw = request.POST.get("age")
    age = _parse_safe_int(age_raw if age_raw and age_raw.strip() else None, 0)
    return JsonResponse({"name": name, "age": age})


async def file_params(request: HttpRequest) -> HttpResponse:
    content_type = request.headers.get("content-type", "").lower()
    if not content_type.startswith("multipart/form-data"):
        return _error(INVALID_MULTIPART, 400, EXPECTED_MULTIPART_CONTENT_TYPE)

    upload = request.FILES.get("file")
    if upload is None:
        return _error(FILE_NOT_FOUND, 400, "no file field named 'file' in form data")

    part_type = (upload.content_type or "").lower()
    if not part_type.startswith("text/plain"):
        return _error(ONLY_TEXT_PLAIN, 415, f"received mimetype: {upload.content_type or 'unknown'}")

    data = upload.read()
    if len(data) > MAX_FILE_BYTES:
        return _error(FILE_SIZE_EXCEEDS, 413, f"file size {len(data)} exceeds limit {MAX_FILE_BYTES}")

    if NULL_BYTE in data[:SNIFF_LEN]:
        return _error(FILE_NOT_TEXT, 415, "file contains null bytes in header")
    if NULL_BYTE in data:
        return _error(FILE_NOT_TEXT, 415, "file contains null bytes")

    try:
        content = data.decode("utf-8")
    except UnicodeDecodeError as e:
        return _error(FILE_NOT_TEXT, 415, e)

    return JsonResponse({"filename": upload.name, "size": len(data), "content": content})
