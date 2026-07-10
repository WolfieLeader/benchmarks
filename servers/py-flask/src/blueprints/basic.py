from __future__ import annotations

from flask import Blueprint, Response, jsonify
from flask.typing import ResponseReturnValue

bp = Blueprint("basic", __name__)


@bp.get("/")
def root() -> ResponseReturnValue:
    return jsonify({"hello": "world"})


@bp.get("/health")
def health() -> ResponseReturnValue:
    return Response("OK", mimetype="text/plain")
