from __future__ import annotations

from django.http import HttpRequest, HttpResponse, JsonResponse


async def root(request: HttpRequest) -> JsonResponse:
    return JsonResponse({"hello": "world"})


async def health(request: HttpRequest) -> HttpResponse:
    return HttpResponse("OK", content_type="text/plain")
