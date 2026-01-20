import logging
import time
from fastapi import FastAPI, HTTPException, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import PlainTextResponse, JSONResponse
from starlette.exceptions import HTTPException as StarletteHTTPException

from src.env import env
from src.routes.params import router as params_router

app = FastAPI(title="FastAPI")


@app.middleware("http")
async def logging_middleware(request: Request, call_next):
    if env.ENV == "prod":
        return await call_next(request)

    start_time = time.perf_counter()
    response = await call_next(request)
    process_time = (time.perf_counter() - start_time) * 1000

    logging.info(
        f"{request.method} {request.url.path} {response.status_code} {process_time:.2f}ms"
    )
    return response


@app.exception_handler(RequestValidationError)
async def validation_exception_handler(request: Request, exc: RequestValidationError):
    return JSONResponse(status_code=400, content={"error": "invalid JSON body"})


@app.exception_handler(StarletteHTTPException)
async def not_found_exception_handler(request: Request, exc: StarletteHTTPException):
    if exc.status_code == 404:
        return JSONResponse(status_code=404, content={"error": "not found"})
    return JSONResponse(status_code=exc.status_code, content={"error": exc.detail})


@app.exception_handler(HTTPException)
async def http_exception_handler(request: Request, exc: HTTPException):
    return JSONResponse(status_code=exc.status_code, content={"error": exc.detail})


@app.exception_handler(Exception)
async def general_exception_handler(request: Request, exc: Exception):
    return JSONResponse(status_code=500, content={"error": str(exc) or "internal error"})


@app.get("/")
def hello_world():
    return {"message": "Hello, World!"}


@app.get("/health", response_class=PlainTextResponse)
def health():
    return "OK"


app.include_router(params_router, prefix="/params")
