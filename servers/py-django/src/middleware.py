from __future__ import annotations

import logging
import time
from collections.abc import Awaitable, Callable

from asgiref.sync import markcoroutinefunction
from django.http import HttpRequest, HttpResponse

logger = logging.getLogger("django.request")


class LoggingMiddleware:
    """Async request logger, installed only under the dev profile (see settings)."""

    async_capable = True
    sync_capable = False

    def __init__(self, get_response: Callable[[HttpRequest], Awaitable[HttpResponse]]) -> None:
        self.get_response = get_response
        markcoroutinefunction(self)

    async def __call__(self, request: HttpRequest) -> HttpResponse:
        start = time.perf_counter()
        response = await self.get_response(request)
        elapsed_ms = (time.perf_counter() - start) * 1000
        logger.info("%s %s %s %.2fms", request.method, request.path, response.status_code, elapsed_ms)
        return response
