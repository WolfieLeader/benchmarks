from fastapi import FastAPI

app = FastAPI()

@app.get("/")
def hello_world():
    return "Hello World!"

@app.get("/ping")
def ping():
    return "PONG!"