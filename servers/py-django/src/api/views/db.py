from __future__ import annotations

import json
from typing import Any

from django.http import HttpRequest, HttpResponse, JsonResponse
from django.views import View

from bench_shared.errors import INTERNAL_ERROR, INVALID_JSON_BODY, NOT_FOUND, make_error
from bench_shared.schemas import CreateUser, UpdateUser
from src.api.repositories.base import UserRepository, resolve_repository


def _error(error: str, status: int, detail: str | Exception | None = None) -> JsonResponse:
    return JsonResponse(make_error(error, detail), status=status)


def _not_found(user_id: str) -> JsonResponse:
    return _error(NOT_FOUND, 404, f"user with id {user_id} not found")


def _require_repo(database: str) -> UserRepository | JsonResponse:
    repo = resolve_repository(database)
    if repo is None:
        return _error(NOT_FOUND, 404, f"unknown database type: {database}")
    return repo


async def database_health(request: HttpRequest, database: str) -> HttpResponse:
    repo = resolve_repository(database)
    if repo is None:
        return HttpResponse("Service Unavailable", status=503, content_type="text/plain")
    if await repo.health_check():
        return HttpResponse("OK", content_type="text/plain")
    return HttpResponse("Service Unavailable", status=503, content_type="text/plain")


async def reset_database(request: HttpRequest, database: str) -> HttpResponse:
    repo = _require_repo(database)
    if isinstance(repo, JsonResponse):
        return repo
    try:
        await repo.delete_all()
    except Exception as e:
        return _error(INTERNAL_ERROR, 500, e)
    return JsonResponse({"status": "ok"})


class UsersCollectionView(View):
    async def post(self, request: HttpRequest, database: str) -> HttpResponse:
        repo = _require_repo(database)
        if isinstance(repo, JsonResponse):
            return repo
        try:
            payload: Any = json.loads(request.body)
            data = CreateUser.model_validate(payload)
        except ValueError as e:
            return _error(INVALID_JSON_BODY, 400, e)
        try:
            user = await repo.create(data)
        except Exception as e:
            return _error(INTERNAL_ERROR, 500, e)
        return JsonResponse(user.model_dump(exclude_none=True), status=201)

    async def delete(self, request: HttpRequest, database: str) -> HttpResponse:
        repo = _require_repo(database)
        if isinstance(repo, JsonResponse):
            return repo
        try:
            await repo.delete_all()
        except Exception as e:
            return _error(INTERNAL_ERROR, 500, e)
        return JsonResponse({"success": True})


class UserDetailView(View):
    async def get(self, request: HttpRequest, database: str, user_id: str) -> HttpResponse:
        repo = _require_repo(database)
        if isinstance(repo, JsonResponse):
            return repo
        try:
            user = await repo.find_by_id(user_id)
        except Exception as e:
            return _error(INTERNAL_ERROR, 500, e)
        if user is None:
            return _not_found(user_id)
        return JsonResponse(user.model_dump(exclude_none=True))

    async def patch(self, request: HttpRequest, database: str, user_id: str) -> HttpResponse:
        repo = _require_repo(database)
        if isinstance(repo, JsonResponse):
            return repo
        try:
            payload: Any = json.loads(request.body)
            data = UpdateUser.model_validate(payload)
        except ValueError as e:
            return _error(INVALID_JSON_BODY, 400, e)
        try:
            user = await repo.update(user_id, data)
        except Exception as e:
            return _error(INTERNAL_ERROR, 500, e)
        if user is None:
            return _not_found(user_id)
        return JsonResponse(user.model_dump(exclude_none=True))

    async def delete(self, request: HttpRequest, database: str, user_id: str) -> HttpResponse:
        repo = _require_repo(database)
        if isinstance(repo, JsonResponse):
            return repo
        try:
            deleted = await repo.delete(user_id)
        except Exception as e:
            return _error(INTERNAL_ERROR, 500, e)
        if not deleted:
            return _not_found(user_id)
        return JsonResponse({"success": True})
