from __future__ import annotations

import hashlib
from datetime import UTC, datetime, timedelta
from pathlib import Path
from typing import Any

import jwt
from fastapi import APIRouter, HTTPException, Request
from fastapi.templating import Jinja2Templates

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

web_router = APIRouter()

_templates = Jinja2Templates(directory=Path(__file__).parent.parent / "templates")
_BEARER_PREFIX = "Bearer "


@web_router.get("/html")
async def html(request: Request):
    # Server-rendered Jinja2 template (text/html) — the FastAPI/Starlette
    # equivalent of Flask's render_template. Canon interpolations: greeting, fruit
    # list, labeled total (contract/web.json). Trivial in-memory render, so it runs
    # inline on the loop; no threadpool hop is warranted (unlike /compute).
    return _templates.TemplateResponse(
        request, "page.html", {"name": "Alice", "fruits": ["apple", "banana", "cherry"], "total": 42}
    )


@web_router.get("/jwt/sign")
async def jwt_sign():
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
    return {"token": token}


@web_router.get("/jwt/verify")
async def jwt_verify(request: Request):
    header = request.headers.get("Authorization", "")
    if not header.startswith(_BEARER_PREFIX):
        raise HTTPException(status_code=401, detail=make_error(INVALID_TOKEN, "missing bearer token"))
    token = header[len(_BEARER_PREFIX) :].strip()
    try:
        # PyJWT verifies the HS256 signature AND the exp claim by default, so a
        # wrong-signature or expired token both raise InvalidTokenError -> 401.
        payload: dict[str, Any] = jwt.decode(token, env.JWT_SECRET, algorithms=["HS256"])  # pyright: ignore[reportUnknownMemberType]
    except jwt.InvalidTokenError as e:
        raise HTTPException(status_code=401, detail=make_error(INVALID_TOKEN, e)) from e
    return payload


@web_router.post("/validate")
async def validate(request: Request):
    # The typed-body-param path would raise RequestValidationError -> the global
    # handler's "invalid JSON body" string, not the canon "validation failed"; so
    # read the raw body and run the shared validator, mirroring Flask/Django.
    detail = validate_payload(await request.body())
    if detail is not None:
        raise HTTPException(status_code=400, detail=make_error(VALIDATION_FAILED, detail))
    return {"valid": True}


@web_router.get("/compute")
def compute(n: str | None = None):
    # Sync def (not async) on purpose: the SHA-256 chain is CPU-bound up to
    # COMPUTE_CAP (1e6) rounds (~sub-second). FastAPI runs plain `def` routes in its
    # threadpool (guide §4.22), so a heavy /compute never freezes the single event
    # loop for other requests; an `async def` here would monopolize the loop for the
    # whole chain with no await point to yield at.
    rounds = parse_compute_rounds(n)
    if rounds is None:
        raise HTTPException(status_code=400, detail=make_error(INVALID_N, "n must be an integer >= 1"))
    rounds = min(rounds, COMPUTE_CAP)
    state = SHA256_SEED
    for _ in range(rounds):
        state = hashlib.sha256(state).digest()
    return {"result": state.hex()}
