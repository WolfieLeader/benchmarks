from fastapi import Request
from fastapi.exceptions import HTTPException
from fastapi.responses import JSONResponse
from starlette.exceptions import HTTPException as StarletteHTTPException
from src.consts.errors import INVALID_JSON_BODY, NOT_FOUND, INTERNAL_ERROR


async def validation_exception_handler(request: Request, exc: Exception):
    details = str(exc) if str(exc) else None
    content = {"error": INVALID_JSON_BODY}
    if details:
        content["details"] = details
    return JSONResponse(status_code=400, content=content)


async def not_found_exception_handler(request: Request, exc: Exception):
    if isinstance(exc, StarletteHTTPException):
        if exc.status_code == 404:
            content = {"error": NOT_FOUND}
            if exc.detail and exc.detail != "Not Found":
                content["details"] = exc.detail
            return JSONResponse(status_code=404, content=content)
        content = {"error": exc.detail}
        return JSONResponse(status_code=exc.status_code, content=content)
    return JSONResponse(status_code=500, content={"error": INTERNAL_ERROR})


async def http_exception_handler(request: Request, exc: Exception):
    if isinstance(exc, HTTPException):
        if isinstance(exc.detail, dict) and "error" in exc.detail:
            return JSONResponse(status_code=exc.status_code, content=exc.detail)
        return JSONResponse(status_code=exc.status_code, content={"error": exc.detail})
    return JSONResponse(status_code=500, content={"error": INTERNAL_ERROR})


async def general_exception_handler(request: Request, exc: Exception):
    details = str(exc) if str(exc) else None
    content = {"error": INTERNAL_ERROR}
    if details:
        content["details"] = details
    return JSONResponse(status_code=500, content=content)
