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
    EXPECTED_FORM_CONTENT_TYPE,
    EXPECTED_MULTIPART_CONTENT_TYPE,
    INVALID_JSON_BODY,
    INVALID_FORM_DATA,
    INVALID_MULTIPART,
    FILE_NOT_FOUND,
    FILE_SIZE_EXCEEDS,
    ONLY_TEXT_PLAIN,
    FILE_NOT_TEXT,
    make_error,
)


params_router = APIRouter()


@params_router.get("/search")
async def search_params(q: str | None = None, limit: str | None = None):
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


@params_router.get("/url/{dynamic}")
async def url_params(dynamic: str):
    return {"dynamic": dynamic}


@params_router.get("/header")
async def header_params(
    header: str | None = Header(alias="X-Custom-Header", default=None),
):
    return {"header": header.strip() if header and header.strip() else "none"}


@params_router.post("/body")
async def body_params(body: Any = Body()):
    if not isinstance(body, dict):
        raise HTTPException(status_code=400, detail=make_error(INVALID_JSON_BODY, "expected a JSON object"))
    return {"body": body}


@params_router.get("/cookie")
async def cookie_params(
    response: Response,
    foo: str | None = Cookie(default=None),
):
    response.set_cookie(key="bar", value="12345", max_age=10, httponly=True, path="/")
    return {"cookie": foo.strip() if foo and foo.strip() else "none"}


@params_router.post("/form")
async def form_params(request: Request):
    content_type = request.headers.get("content-type", "").lower()
    if not (
        content_type.startswith("application/x-www-form-urlencoded") or content_type.startswith("multipart/form-data")
    ):
        raise HTTPException(
            status_code=400,
            detail=make_error(INVALID_FORM_DATA, EXPECTED_FORM_CONTENT_TYPE),
        )

    try:
        form = await request.form()
    except Exception as e:
        raise HTTPException(
            status_code=400, detail=make_error(INVALID_FORM_DATA, str(e) or "failed to parse form body")
        )

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


@params_router.post("/file")
async def file_params(request: Request, file: UploadFile | None = File(default=None)):
    content_type = request.headers.get("content-type", "").lower()
    if not content_type.startswith("multipart/form-data"):
        raise HTTPException(status_code=400, detail=make_error(INVALID_MULTIPART, EXPECTED_MULTIPART_CONTENT_TYPE))

    if file is None:
        raise HTTPException(
            status_code=400, detail=make_error(FILE_NOT_FOUND, "no file field named 'file' in form data")
        )

    if not file.content_type or not file.content_type.startswith("text/plain"):
        raise HTTPException(
            status_code=415, detail=make_error(ONLY_TEXT_PLAIN, f"received mimetype: {file.content_type or 'unknown'}")
        )

    data = await file.read(MAX_FILE_BYTES + 1)
    if len(data) > MAX_FILE_BYTES:
        raise HTTPException(
            status_code=413,
            detail=make_error(FILE_SIZE_EXCEEDS, f"file size {len(data)} exceeds limit {MAX_FILE_BYTES}"),
        )

    head = data[:SNIFF_LEN]
    if NULL_BYTE in head:
        raise HTTPException(
            status_code=415,
            detail=make_error(FILE_NOT_TEXT, "file contains null bytes in header"),
        )

    if NULL_BYTE in data:
        raise HTTPException(
            status_code=415,
            detail=make_error(FILE_NOT_TEXT, "file contains null bytes"),
        )

    try:
        content = data.decode("utf-8")
    except UnicodeDecodeError:
        raise HTTPException(
            status_code=415,
            detail=make_error(FILE_NOT_TEXT, "file is not valid UTF-8"),
        )

    return {
        "filename": file.filename,
        "size": len(data),
        "content": content,
    }
