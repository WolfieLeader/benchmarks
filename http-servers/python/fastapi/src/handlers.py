from fastapi import Request
from fastapi.exceptions import HTTPException
from fastapi.responses import JSONResponse
from starlette.exceptions import HTTPException as StarletteHTTPException

from src.consts.errors import INTERNAL_ERROR, INVALID_JSON_BODY, NOT_FOUND, make_error


async def validation_exception_handler(request: Request, exc: Exception):
    return JSONResponse(status_code=400, content=make_error(INVALID_JSON_BODY, exc))


async def not_found_exception_handler(request: Request, exc: Exception):
    if isinstance(exc, StarletteHTTPException):
        if exc.status_code == 404:
            detail = exc.detail if exc.detail and exc.detail != "Not Found" else None
            return JSONResponse(status_code=404, content=make_error(NOT_FOUND, detail))
        return JSONResponse(status_code=exc.status_code, content={"error": exc.detail})
    return JSONResponse(status_code=500, content=make_error(INTERNAL_ERROR))


async def http_exception_handler(request: Request, exc: Exception):
    if isinstance(exc, HTTPException):
        if isinstance(exc.detail, dict) and "error" in exc.detail:
            return JSONResponse(status_code=exc.status_code, content=exc.detail)
        return JSONResponse(status_code=exc.status_code, content={"error": exc.detail})
    return JSONResponse(status_code=500, content=make_error(INTERNAL_ERROR))


async def general_exception_handler(request: Request, exc: Exception):
    return JSONResponse(status_code=500, content=make_error(INTERNAL_ERROR, exc))
