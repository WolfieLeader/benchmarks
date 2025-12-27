from fastapi import FastAPI

app = FastAPI(title="FastAPI")


@app.get("/")
def hello_world():
    return {"message": "Hello, World!"}
