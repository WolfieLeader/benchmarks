from fastapi.responses import PlainTextResponse

from fastapi import FastAPI

app = FastAPI(title="FastAPI")


@app.get("/")
def hello_world():
    return {"message": "Hello, World!"}


@app.get("/ping", response_class=PlainTextResponse)
def ping():
    return "PONG"
