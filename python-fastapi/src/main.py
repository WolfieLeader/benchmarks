from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse, PlainTextResponse


from src.routes.params import router as params_router

app = FastAPI(title="FastAPI")


@app.exception_handler(RequestValidationError)
async def validation_exception_handler(request: Request, exc: RequestValidationError):
    # This will also turn other validation errors into 400, which matches your "just 400" policy.
    return JSONResponse(status_code=400, content={"error": "invalid JSON body"})


@app.get("/")
def hello_world():
    return {"message": "Hello, World!"}


@app.get("/health", response_class=PlainTextResponse)
def health():
    return "OK"


app.include_router(params_router, prefix="/params")
