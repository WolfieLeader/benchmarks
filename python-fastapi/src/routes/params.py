from fastapi import APIRouter, Body, Cookie, File, Header, Request, Response, UploadFile
from fastapi.exceptions import HTTPException
from typing import Any
from src.consts.defaults import (
    MAX_FILE_BYTES,
    NULL_BYTE,
    SNIFF_LEN,
    SAFE_INT_LIMIT,
    DEFAULT_LIMIT,
)
from src.consts.errors import (
    INVALID_JSON_BODY,
    INVALID_FORM_DATA,
    INVALID_MULTIPART,
    FILE_NOT_FOUND,
    FILE_SIZE_EXCEEDS,
    ONLY_TEXT_PLAIN,
    FILE_NOT_TEXT,
)


router = APIRouter()


@router.get("/search")
def search_params(q: str | None = None, limit: str | None = None):
    search = q.strip() if q is not None and q.strip() else "none"
    parsed_limit = DEFAULT_LIMIT

    if limit is not None:
        if "." not in limit:
            try:
                num = int(limit)
                if -SAFE_INT_LIMIT <= num <= SAFE_INT_LIMIT:
                    parsed_limit = num
            except ValueError:
                pass

    return {"search": search, "limit": parsed_limit}


@router.get("/url/{dynamic}")
def url_params(dynamic: str):
    return {"dynamic": dynamic}


@router.get("/header")
def header_params(header: str | None = Header(alias="X-Custom-Header", default=None)):
    return {"header": header.strip() if header and header.strip() else "none"}


@router.post("/body")
def body_params(body: Any = Body()):
    if not isinstance(body, dict):
        raise HTTPException(status_code=400, detail=INVALID_JSON_BODY)
    return {"body": body}


@router.get("/cookie")
def cookie_params(
    response: Response,
    foo: str | None = Cookie(default=None),
):
    response.set_cookie(key="bar", value="12345", max_age=10, httponly=True, path="/")
    return {"cookie": foo.strip() if foo and foo.strip() else "none"}


@router.post("/form")
async def form_params(request: Request):
    content_type = request.headers.get("content-type", "").lower()
    if not (
        content_type.startswith("application/x-www-form-urlencoded")
        or content_type.startswith("multipart/form-data")
    ):
        raise HTTPException(status_code=400, detail=INVALID_FORM_DATA)

    try:
        form = await request.form()
    except Exception:
        raise HTTPException(status_code=400, detail=INVALID_FORM_DATA)

    name_val = form.get("name")
    name = name_val.strip() if isinstance(name_val, str) else ""
    if name == "":
        name = "none"

    age_val = form.get("age")
    age = 0
    if isinstance(age_val, str) and age_val.strip() != "":
        try:
            num = int(age_val)
            if -SAFE_INT_LIMIT <= num <= SAFE_INT_LIMIT:
                age = num
        except ValueError:
            pass

    return {"name": name, "age": age}


@router.post("/file")
async def file_params(request: Request, file: UploadFile | None = File(default=None)):
    content_type = request.headers.get("content-type", "").lower()
    if not content_type.startswith("multipart/form-data"):
        raise HTTPException(status_code=400, detail=INVALID_MULTIPART)

    if file is None:
        raise HTTPException(status_code=400, detail=FILE_NOT_FOUND)

    if file.content_type and not file.content_type.startswith("text/plain"):
        raise HTTPException(status_code=415, detail=ONLY_TEXT_PLAIN)

    data = await file.read(MAX_FILE_BYTES + 1)
    if len(data) > MAX_FILE_BYTES:
        raise HTTPException(status_code=413, detail=FILE_SIZE_EXCEEDS)

    head = data[:SNIFF_LEN]
    if NULL_BYTE in head or NULL_BYTE in data:
        raise HTTPException(
            status_code=415,
            detail=FILE_NOT_TEXT,
        )

    try:
        content = data.decode("utf-8")
    except UnicodeDecodeError:
        raise HTTPException(
            status_code=415,
            detail=FILE_NOT_TEXT,
        )

    return {
        "filename": file.filename,
        "size": len(data),
        "content": content,
    }
