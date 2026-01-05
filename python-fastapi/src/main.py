from fastapi import FastAPI
from fastapi.responses import PlainTextResponse

app = FastAPI(title="FastAPI")


@app.get("/")
def hello_world():
    return {"message": "Hello, World!"}


@app.get("/health", response_class=PlainTextResponse)
def health():
    return "OK"
