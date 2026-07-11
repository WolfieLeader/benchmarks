from __future__ import annotations

import asyncio
import hashlib
from datetime import UTC, datetime, timedelta
from typing import Any

import jwt
from django.http import HttpRequest, HttpResponse, JsonResponse
from django.shortcuts import render

from bench_shared.env import env
from bench_shared.errors import INVALID_N, INVALID_TOKEN, VALIDATION_FAILED, make_error
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

_BEARER_PREFIX = "Bearer "


def _error(error: str, status: int, detail: str | Exception | None = None) -> JsonResponse:
    return JsonResponse(make_error(error, detail), status=status)


async def html(request: HttpRequest) -> HttpResponse:
    # Server-rendered via Django's own template engine (render -> text/html), the
    # framework-idiomatic parallel to Flask's Jinja2 render_template. The render is
    # a trivial in-memory string build (no ORM/I/O), so it is safe to call inline in
    # this async view. Canon interpolations per contract/web.json.
    return render(request, "page.html", {"name": "Alice", "fruits": ["apple", "banana", "cherry"], "total": 42})


async def jwt_sign(request: HttpRequest) -> JsonResponse:
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
    return JsonResponse({"token": token})


async def jwt_verify(request: HttpRequest) -> JsonResponse:
    header = request.headers.get("Authorization", "")
    if not header.startswith(_BEARER_PREFIX):
        return _error(INVALID_TOKEN, 401, "missing bearer token")
    token = header[len(_BEARER_PREFIX) :].strip()
    try:
        # PyJWT verifies the HS256 signature AND the exp claim by default, so a
        # wrong-signature or expired token both raise InvalidTokenError -> 401.
        payload: dict[str, Any] = jwt.decode(token, env.JWT_SECRET, algorithms=["HS256"])  # pyright: ignore[reportUnknownMemberType]
    except jwt.InvalidTokenError as e:
        return _error(INVALID_TOKEN, 401, e)
    return JsonResponse(payload)


async def validate(request: HttpRequest) -> JsonResponse:
    detail = validate_payload(request.body)
    if detail is not None:
        return _error(VALIDATION_FAILED, 400, detail)
    return JsonResponse({"valid": True})


def _compute_chain(rounds: int) -> str:
    state = SHA256_SEED
    for _ in range(rounds):
        state = hashlib.sha256(state).digest()
    return state.hex()


async def compute(request: HttpRequest) -> JsonResponse:
    rounds = parse_compute_rounds(request.GET.get("n"))
    if rounds is None:
        return _error(INVALID_N, 400, "n must be an integer >= 1")
    rounds = min(rounds, COMPUTE_CAP)
    # The chain is CPU-bound up to COMPUTE_CAP (1e6) rounds (~sub-second). Django
    # async views run on the single event loop, so bridge the sync work off it with
    # asyncio.to_thread (guide §2.10) — an inline loop would freeze every concurrent
    # request. The other web views do only microsecond CPU and run inline.
    digest = await asyncio.to_thread(_compute_chain, rounds)
    return JsonResponse({"result": digest})
