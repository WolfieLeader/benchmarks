import logging
import time
from contextlib import asynccontextmanager

import uvicorn
from fastapi import FastAPI, HTTPException
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse, PlainTextResponse
from starlette.exceptions import HTTPException as StarletteHTTPException
from starlette.requests import Request

from src.config.env import env
from src.consts.defaults import MAX_REQUEST_BYTES
from src.consts.errors import REQUEST_TOO_LARGE, make_error
from src.database.repository import (
    disconnect_databases,
    initialize_databases,
)
from src.handlers import (
    general_exception_handler,
    http_exception_handler,
    not_found_exception_handler,
    validation_exception_handler,
)
from src.routes.db import db_router
from src.routes.params import params_router


@asynccontextmanager
async def lifespan(app: FastAPI):
    await initialize_databases()
    yield
    await disconnect_databases()


app = FastAPI(title="FastAPI", lifespan=lifespan)


@app.middleware("http")
async def body_size_limit_middleware(request: Request, call_next):
    # Global request-body cap so no route can read an unbounded body. The file
    # route enforces its own smaller 1MB limit; a body under this global cap
    # still reaches that check and returns its own 413.
    content_length = request.headers.get("content-length")
    if content_length is not None:
        try:
            too_large = int(content_length) > MAX_REQUEST_BYTES
        except ValueError:
            too_large = False
        if too_large:
            return JSONResponse(status_code=413, content=make_error(REQUEST_TOO_LARGE))
    return await call_next(request)


@app.middleware("http")
async def logging_middleware(request: Request, call_next):
    if env.ENV == "prod":
        return await call_next(request)

    start_time = time.perf_counter()
    response = await call_next(request)
    process_time = (time.perf_counter() - start_time) * 1000

    logging.info(f"{request.method} {request.url.path} {response.status_code} {process_time:.2f}ms")
    return response


app.add_exception_handler(RequestValidationError, validation_exception_handler)
app.add_exception_handler(StarletteHTTPException, not_found_exception_handler)
app.add_exception_handler(HTTPException, http_exception_handler)
app.add_exception_handler(Exception, general_exception_handler)


@app.get("/")
async def root():
    return {"hello": "world"}


@app.get("/health")
async def health():
    return PlainTextResponse("OK")


app.include_router(params_router, prefix="/params")
app.include_router(db_router, prefix="/db")


if __name__ == "__main__":
    uvicorn.run(app, host=env.HOST, port=env.PORT)
