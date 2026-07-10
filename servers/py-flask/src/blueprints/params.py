from __future__ import annotations

import json
from typing import Any

from flask import Blueprint, jsonify, request
from flask.typing import ResponseReturnValue

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
)
from src.responses import json_error

bp = Blueprint("params", __name__)

_FORM_CONTENT_TYPES = ("application/x-www-form-urlencoded", "multipart/form-data")


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


@bp.get("/search")
def search_params() -> ResponseReturnValue:
    return jsonify(
        {
            "search": _strip_or(request.args.get("q"), "none"),
            "limit": _parse_safe_int(request.args.get("limit"), DEFAULT_LIMIT),
        }
    )


@bp.get("/url/<dynamic>")
def url_params(dynamic: str) -> ResponseReturnValue:
    return jsonify({"dynamic": dynamic})


@bp.get("/header")
def header_params() -> ResponseReturnValue:
    return jsonify({"header": _strip_or(request.headers.get("X-Custom-Header"), "none")})


@bp.post("/body")
def body_params() -> ResponseReturnValue:
    try:
        payload: Any = json.loads(request.get_data())
    except ValueError as e:
        return json_error(INVALID_JSON_BODY, 400, e)
    if not isinstance(payload, dict):
        return json_error(INVALID_JSON_BODY, 400, "expected a JSON object")
    return jsonify({"body": payload})


@bp.get("/cookie")
def cookie_params() -> ResponseReturnValue:
    response = jsonify({"cookie": _strip_or(request.cookies.get("foo"), "none")})
    response.set_cookie("bar", "12345", max_age=10, httponly=True, path="/")
    return response


@bp.post("/form")
def form_params() -> ResponseReturnValue:
    if request.mimetype not in _FORM_CONTENT_TYPES:
        return json_error(INVALID_FORM_DATA, 400, EXPECTED_FORM_CONTENT_TYPE)

    name = _strip_or(request.form.get("name"), "none")
    age_raw = request.form.get("age")
    age = _parse_safe_int(age_raw if age_raw and age_raw.strip() else None, 0)
    return jsonify({"name": name, "age": age})


@bp.post("/file")
def file_params() -> ResponseReturnValue:
    if request.mimetype != "multipart/form-data":
        return json_error(INVALID_MULTIPART, 400, EXPECTED_MULTIPART_CONTENT_TYPE)

    upload = request.files.get("file")
    if upload is None:
        return json_error(FILE_NOT_FOUND, 400, "no file field named 'file' in form data")

    part_type = (upload.mimetype or "").lower()
    if not part_type.startswith("text/plain"):
        return json_error(ONLY_TEXT_PLAIN, 415, f"received mimetype: {upload.mimetype or 'unknown'}")

    data = upload.read()
    if len(data) > MAX_FILE_BYTES:
        return json_error(FILE_SIZE_EXCEEDS, 413, f"file size {len(data)} exceeds limit {MAX_FILE_BYTES}")

    if NULL_BYTE in data[:SNIFF_LEN]:
        return json_error(FILE_NOT_TEXT, 415, "file contains null bytes in header")
    if NULL_BYTE in data:
        return json_error(FILE_NOT_TEXT, 415, "file contains null bytes")

    try:
        content = data.decode("utf-8")
    except UnicodeDecodeError as e:
        return json_error(FILE_NOT_TEXT, 415, e)

    return jsonify({"filename": upload.filename, "size": len(data), "content": content})
