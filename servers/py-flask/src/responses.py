from __future__ import annotations

from flask import Response, jsonify

from bench_shared.errors import make_error


def json_error(error: str, status: int, detail: str | Exception | None = None) -> tuple[Response, int]:
    """Canonical error response: {"error": ..., "details"?: ...} + the given status.

    `details` is omitted (never null) when there is nothing to add — the shared
    make_error enforces the fleet-wide error shape.
    """
    return jsonify(make_error(error, detail)), status
