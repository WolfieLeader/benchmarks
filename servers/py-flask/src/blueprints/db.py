from __future__ import annotations

import json
from typing import Any

from flask import Blueprint, Response, jsonify, request
from flask.typing import ResponseReturnValue

from bench_shared.errors import INTERNAL_ERROR, INVALID_JSON_BODY, NOT_FOUND
from bench_shared.schemas import CreateUser, UpdateUser
from src.repositories.base import UserRepository, resolve_repository
from src.responses import json_error

bp = Blueprint("db", __name__)


def _not_found(user_id: str) -> tuple[Response, int]:
    return json_error(NOT_FOUND, 404, f"user with id {user_id} not found")


def _require_repo(database: str) -> UserRepository | tuple[Response, int]:
    repo = resolve_repository(database)
    if repo is None:
        return json_error(NOT_FOUND, 404, f"unknown database type: {database}")
    return repo


@bp.get("/<database>/health")
def database_health(database: str) -> ResponseReturnValue:
    repo = resolve_repository(database)
    if repo is None:
        return Response("Service Unavailable", status=503, mimetype="text/plain")
    try:
        ok = repo.health_check()
    except Exception:
        ok = False
    if ok:
        return Response("OK", mimetype="text/plain")
    return Response("Service Unavailable", status=503, mimetype="text/plain")


@bp.delete("/<database>/reset")
def reset_database(database: str) -> ResponseReturnValue:
    repo = _require_repo(database)
    if isinstance(repo, tuple):
        return repo
    try:
        repo.delete_all()
    except Exception as e:
        return json_error(INTERNAL_ERROR, 500, e)
    return jsonify({"status": "ok"})


@bp.post("/<database>/users")
def create_user(database: str) -> ResponseReturnValue:
    repo = _require_repo(database)
    if isinstance(repo, tuple):
        return repo
    try:
        payload: Any = json.loads(request.get_data())
        data = CreateUser.model_validate(payload)
    except ValueError as e:
        return json_error(INVALID_JSON_BODY, 400, e)
    try:
        user = repo.create(data)
    except Exception as e:
        return json_error(INTERNAL_ERROR, 500, e)
    return jsonify(user.model_dump(exclude_none=True)), 201


@bp.delete("/<database>/users")
def delete_all_users(database: str) -> ResponseReturnValue:
    repo = _require_repo(database)
    if isinstance(repo, tuple):
        return repo
    try:
        repo.delete_all()
    except Exception as e:
        return json_error(INTERNAL_ERROR, 500, e)
    return jsonify({"success": True})


@bp.get("/<database>/users/<user_id>")
def read_user(database: str, user_id: str) -> ResponseReturnValue:
    repo = _require_repo(database)
    if isinstance(repo, tuple):
        return repo
    try:
        user = repo.find_by_id(user_id)
    except Exception as e:
        return json_error(INTERNAL_ERROR, 500, e)
    if user is None:
        return _not_found(user_id)
    return jsonify(user.model_dump(exclude_none=True))


@bp.patch("/<database>/users/<user_id>")
def update_user(database: str, user_id: str) -> ResponseReturnValue:
    repo = _require_repo(database)
    if isinstance(repo, tuple):
        return repo
    try:
        payload: Any = json.loads(request.get_data())
        data = UpdateUser.model_validate(payload)
    except ValueError as e:
        return json_error(INVALID_JSON_BODY, 400, e)
    try:
        user = repo.update(user_id, data)
    except Exception as e:
        return json_error(INTERNAL_ERROR, 500, e)
    if user is None:
        return _not_found(user_id)
    return jsonify(user.model_dump(exclude_none=True))


@bp.delete("/<database>/users/<user_id>")
def delete_user(database: str, user_id: str) -> ResponseReturnValue:
    repo = _require_repo(database)
    if isinstance(repo, tuple):
        return repo
    try:
        deleted = repo.delete(user_id)
    except Exception as e:
        return json_error(INTERNAL_ERROR, 500, e)
    if not deleted:
        return _not_found(user_id)
    return jsonify({"success": True})
