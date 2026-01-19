from fastapi import (
    APIRouter,
    Cookie,
    File,
    Form,
    Header,
    HTTPException,
    Response,
    UploadFile,
    Body,
)
from typing import Any

max_file_bytes = 1 << 20
null_byte = b"\x00"
sniff_len = 512


router = APIRouter()


@router.get("/search")
def search_params(q: str | None = None, limit: str | None = None):
    parsed_limit = 10

    if limit:
        try:
            parsed_limit = int(limit)
        except ValueError:
            parsed_limit = 10

    return {"search": q or "none", "limit": parsed_limit}


@router.get("/url/{dynamic}")
def url_params(dynamic: str):
    return {"dynamic": dynamic}


@router.get("/header")
def header_params(header: str | None = Header(alias="X-Custom-Header", default=None)):
    return {"header": header or "none"}


@router.post("/body")
def body_params(body: Any = Body()):
    if not isinstance(body, dict):
        raise HTTPException(status_code=400, detail="invalid JSON body")
    return {"body": body}


@router.get("/cookie")
def cookie_params(
    response: Response,
    foo: str | None = Cookie(default=None),
):
    response.set_cookie(key="bar", value="12345", max_age=10, httponly=True, path="/")
    return {"cookie": foo or "none"}


@router.post("/form")
def form_params(
    name: str | None = Form(default=None),
    ageStr: str | None = Form(alias="age", default=None),
):
    if name and name.strip() == "":
        name = "none"

    age = 0
    if ageStr:
        try:
            age = int(ageStr)
        except ValueError:
            age = 0

    return {"name": name or "none", "age": age}


@router.post("/file")
async def file_params(file: UploadFile | None = File(default=None)):
    if file is None:
        raise HTTPException(status_code=400, detail="file not found in form data")

    if file.content_type and not file.content_type.startswith("text/plain"):
        raise HTTPException(status_code=415, detail="only text/plain files are allowed")

    data = await file.read(max_file_bytes + 1)
    if len(data) > max_file_bytes:
        raise HTTPException(status_code=413, detail="file size exceeds limit")

    head = data[:sniff_len]
    if null_byte in head:
        raise HTTPException(
            status_code=415,
            detail="file does not look like plain text",
        )

    if null_byte in data:
        raise HTTPException(
            status_code=415,
            detail="file does not look like plain text",
        )

    try:
        content = data.decode("utf-8")
    except UnicodeDecodeError as exc:
        raise HTTPException(
            status_code=415,
            detail="file does not look like plain text",
        ) from exc

    return {
        "filename": file.filename,
        "size": len(data),
        "content": content,
    }
