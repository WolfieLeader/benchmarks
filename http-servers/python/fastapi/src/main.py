import logging
import time
from contextlib import asynccontextmanager

import uvicorn
from fastapi import FastAPI, HTTPException
from fastapi.exceptions import RequestValidationError
from fastapi.responses import PlainTextResponse
from starlette.exceptions import HTTPException as StarletteHTTPException
from starlette.requests import Request

from src.config.env import env
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
def root():
    return {"hello": "world"}


@app.get("/health")
def health():
    return PlainTextResponse("OK")


app.include_router(params_router, prefix="/params")
app.include_router(db_router, prefix="/db")


if __name__ == "__main__":
    uvicorn.run(app, host=env.HOST, port=env.PORT)
