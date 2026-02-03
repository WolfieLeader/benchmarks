import asyncio
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
    DATABASE_TYPES,
    disconnect_databases,
    get_all_repositories,
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
    return PlainTextResponse("OK")


@app.get("/health")
async def health():
    repositories = get_all_repositories()

    async def check_db(db_type: str) -> tuple[str, str]:
        repo = repositories.get(db_type)  # type: ignore[arg-type]
        if repo is None:
            return db_type, "unhealthy"
        try:
            healthy = await repo.health_check()
            return db_type, "healthy" if healthy else "unhealthy"
        except Exception:
            return db_type, "unhealthy"

    results = await asyncio.gather(*[check_db(db_type) for db_type in DATABASE_TYPES])
    db_statuses = dict(results)

    return {
        "status": "healthy",
        "databases": db_statuses,
    }


app.include_router(params_router, prefix="/params")
app.include_router(db_router, prefix="/db")


if __name__ == "__main__":
    uvicorn.run(app, host=env.HOST, port=env.PORT)
