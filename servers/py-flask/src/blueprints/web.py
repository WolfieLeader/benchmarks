from __future__ import annotations

import hashlib
from datetime import UTC, datetime, timedelta
from typing import Any

import jwt
from flask import Blueprint, jsonify, render_template, request
from flask.typing import ResponseReturnValue

from bench_shared.env import env
from bench_shared.errors import INVALID_N, INVALID_TOKEN, VALIDATION_FAILED
from bench_shared.web import (
    COMPUTE_CAP,
    JWT_ADMIN,
    JWT_NAME,
    JWT_SUBJECT,
    JWT_TTL_SECONDS,
    SHA256_SEED,
    parse_compute_rounds,
    validate_payload,
)
from src.responses import json_error

bp = Blueprint("web", __name__)

_BEARER_PREFIX = "Bearer "


@bp.get("/html")
def html() -> ResponseReturnValue:
    # Server-rendered Jinja2 template; render_template returns text/html. Canon
    # interpolations: greeting, fruit list, labeled total (contract/web.json).
    return render_template("page.html", name="Alice", fruits=["apple", "banana", "cherry"], total=42)


@bp.get("/jwt/sign")
def jwt_sign() -> ResponseReturnValue:
    now = datetime.now(UTC)
    payload: dict[str, Any] = {
        "sub": JWT_SUBJECT,
        "name": JWT_NAME,
        "admin": JWT_ADMIN,
        "iat": int(now.timestamp()),
        "exp": int((now + timedelta(seconds=JWT_TTL_SECONDS)).timestamp()),
    }
    # PyJWT's encode/decode carry a partially-unknown `key` param (its optional
    # cryptography backend types are Unknown without that extra install); pin the
    # single call site with a narrow rule ignore (§7.42) — we only ever use HS256.
    token = jwt.encode(payload, env.JWT_SECRET, algorithm="HS256")  # pyright: ignore[reportUnknownMemberType]
    return jsonify({"token": token})


@bp.get("/jwt/verify")
def jwt_verify() -> ResponseReturnValue:
    header = request.headers.get("Authorization", "")
    if not header.startswith(_BEARER_PREFIX):
        return json_error(INVALID_TOKEN, 401, "missing bearer token")
    token = header[len(_BEARER_PREFIX) :].strip()
    try:
        # PyJWT verifies the HS256 signature AND the exp claim by default, so a
        # wrong-signature or expired token both raise InvalidTokenError -> 401.
        payload: dict[str, Any] = jwt.decode(token, env.JWT_SECRET, algorithms=["HS256"])  # pyright: ignore[reportUnknownMemberType]
    except jwt.InvalidTokenError as e:
        return json_error(INVALID_TOKEN, 401, e)
    return jsonify(payload)


@bp.post("/validate")
def validate() -> ResponseReturnValue:
    detail = validate_payload(request.get_data())
    if detail is not None:
        return json_error(VALIDATION_FAILED, 400, detail)
    return jsonify({"valid": True})


@bp.get("/compute")
def compute() -> ResponseReturnValue:
    n = parse_compute_rounds(request.args.get("n"))
    if n is None:
        return json_error(INVALID_N, 400, "n must be an integer >= 1")
    n = min(n, COMPUTE_CAP)
    state = SHA256_SEED
    for _ in range(n):
        state = hashlib.sha256(state).digest()
    return jsonify({"result": state.hex()})
