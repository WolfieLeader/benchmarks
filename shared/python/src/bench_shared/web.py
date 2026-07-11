"""Web-suite infrastructure shared across every Python server that implements it.

Promoted here under the multi-consumer rule (docs/languages/python.md §10; PLAN §3)
once py-fastapi, py-flask and py-django all set ``web: true`` — three consumers of
the same contract canon. Only framework-independent contract values live here: the
/compute and /jwt/sign constants and the /validate pydantic schema. The handlers
themselves (rendering, signing, request/response shaping) stay per-framework and
idiomatic — that is the idiom boundary (PLAN §3).

Contract: contract/web.json.
"""

from __future__ import annotations

import re
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, EmailStr, Field, ValidationError

# The canon /compute parser is Go's strconv.Atoi (servers/go-stdlib): optional
# leading +/- then ASCII digits only, parsed as a signed 64-bit int. `[0-9]` is
# ASCII-only by construction, so this rejects the underscore separators (PEP-515)
# and Unicode digits that Python's int() would otherwise accept, and the absence
# of any strip() rejects surrounding whitespace that Atoi also rejects.
_ATOI_RE = re.compile(r"[+-]?[0-9]+")
_I64_MAX = 2**63 - 1

# Compute canon: GET /compute applies SHA-256 to SHA256_SEED n times and returns
# the lowercase-hex digest. n must be an integer in [1, COMPUTE_CAP]; above the cap
# it is clamped (bounds the per-request CPU work). SHA256_SEED must equal the
# conformance runner's $sha256chain seed (benchmark/internal/conformance).
SHA256_SEED = b"benchmark"
COMPUTE_CAP = 1_000_000

# JWT canon: GET /jwt/sign issues an HS256 token with these fixed claims plus a
# dynamic iat and exp (= iat + JWT_TTL_SECONDS), signed with the shared JWT_SECRET.
JWT_SUBJECT = "1234567890"
JWT_NAME = "John Doe"
JWT_ADMIN = True
# 1 hour: long enough a token never expires between the contract's sign and verify
# steps; the contract asserts only that exp is present and unexpired ($jwt).
JWT_TTL_SECONDS = 3600


# /validate schema (~4 levels, contract/web.json):
#   user{ id:uuid, email, profile{ age:0..120, role:admin|user|guest,
#         preferences{ theme:light|dark, notifications:bool } } },
#   items[]{ sku, quantity:1..100, tags:string[] }, total:>=0
class _Preferences(BaseModel):
    theme: Literal["light", "dark"]
    notifications: bool


class _Profile(BaseModel):
    # Canon parity (Go reference schema): age carries range rules but no
    # `required`, so an omitted age validates as Go's zero value 0.
    age: int = Field(default=0, ge=0, le=120)
    role: Literal["admin", "user", "guest"]
    preferences: _Preferences


class _ValidateUser(BaseModel):
    id: UUID
    email: EmailStr
    profile: _Profile


class _Item(BaseModel):
    # Canon parity: Go's `required` rejects the empty string.
    sku: str = Field(min_length=1)
    quantity: int = Field(ge=1, le=100)
    # Canon parity: tags carries no presence rule, so an omitted tags decodes to
    # Go's zero value (empty list) and validates.
    tags: list[str] = Field(default_factory=list)


class _ValidatePayload(BaseModel):
    user: _ValidateUser
    # Canon parity: Go's `required,min=1` rejects an empty items array.
    items: list[_Item] = Field(min_length=1)
    # Canon parity: total carries range rules but no `required`, so an omitted
    # total validates as Go's zero value 0.
    total: float = Field(default=0, ge=0)


def parse_compute_rounds(value: str | None) -> int | None:
    """Parse the /compute ``n`` query parameter: an integer >= 1, else None.

    Shared by all three Python servers (multi-consumer rule — same trigger as the
    schema above). The canon is the Go reference's strconv.Atoi (signed 64-bit,
    ASCII digits, no trimming): Python's int() diverges by accepting whitespace,
    PEP-515 underscores ("1_000"), and Unicode digits (Arabic-Indic five,
    U+0665), and by being unbounded, so we gate on an ASCII-only regex and an
    explicit i64 range check before parsing. The caller clamps the result to
    COMPUTE_CAP.
    """
    if value is None or _ATOI_RE.fullmatch(value) is None:
        return None
    n = int(value)
    return n if 1 <= n <= _I64_MAX else None


def validate_payload(raw: bytes) -> str | None:
    """Validate the raw request body against the /validate schema.

    Returns None when the payload is valid, else a short per-framework error
    summary for the canonical ``{"error": "validation failed", "details": ...}``
    response (the exact count is intentionally not asserted by the contract —
    validators count failures differently). One compiled pydantic-core pass parses
    JSON and validates together (model_validate_json).
    """
    try:
        _ValidatePayload.model_validate_json(raw)
    except ValidationError as e:
        count = e.error_count()
        return f"{count} validation error{'s' if count != 1 else ''}"
    return None
