from fastapi import APIRouter, Body, Cookie, File, Header, Request, Response, UploadFile
from fastapi.exceptions import HTTPException
from typing import Any

max_file_bytes = 1 << 20
null_byte = b"\x00"
sniff_len = 512


router = APIRouter()


@router.get("/search")
def search_params(q: str | None = None, limit: str | None = None):
    search = q if q is not None else "none"
    parsed_limit = 10

    if limit is not None:
        if "." not in limit:
            try:
                num = int(limit)
                if -(2**53 - 1) <= num <= (2**53 - 1):
                    parsed_limit = num
            except ValueError:
                pass

    return {"search": search, "limit": parsed_limit}


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
async def form_params(request: Request):
    content_type = request.headers.get("content-type", "").lower()
    if not (
        content_type.startswith("application/x-www-form-urlencoded")
        or content_type.startswith("multipart/form-data")
    ):
        raise HTTPException(status_code=400, detail="invalid form data")

    try:
        form = await request.form()
    except Exception:
        raise HTTPException(status_code=400, detail="invalid form data")

    name_val = form.get("name")
    name = name_val.strip() if isinstance(name_val, str) else ""
    if name == "":
        name = "none"

    age_val = form.get("age")
    age = 0
    if isinstance(age_val, str) and "." not in age_val:
        try:
            num = int(age_val)
            if -(2**53 - 1) <= num <= (2**53 - 1):
                age = num
        except ValueError:
            pass

    return {"name": name, "age": age}


@router.post("/file")
async def file_params(request: Request, file: UploadFile | None = File(default=None)):
    content_type = request.headers.get("content-type", "").lower()
    if not content_type.startswith("multipart/form-data"):
        raise HTTPException(status_code=400, detail="invalid multipart form data")

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
    except UnicodeDecodeError:
        raise HTTPException(
            status_code=415,
            detail="file does not look like plain text",
        )

    return {
        "filename": file.filename,
        "size": len(data),
        "content": content,
    }
