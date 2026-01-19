from fastapi import FastAPI, HTTPException, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import PlainTextResponse, JSONResponse


from src.routes.params import router as params_router

app = FastAPI(title="FastAPI")


@app.exception_handler(RequestValidationError)
async def validation_exception_handler(request: Request, exc: RequestValidationError):
    return JSONResponse(status_code=400, content="Invalid JSON body")


@app.exception_handler(HTTPException)
async def http_exception_handler(request: Request, exc: HTTPException):
    return JSONResponse(status_code=exc.status_code, content={"error": exc.detail})


@app.get("/")
def hello_world():
    return {"message": "Hello, World!"}


@app.get("/health", response_class=PlainTextResponse)
def health():
    return "OK"


app.include_router(params_router, prefix="/params")
