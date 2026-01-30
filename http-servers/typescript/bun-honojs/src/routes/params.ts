import { Hono } from "hono";
import { getCookie, setCookie } from "hono/cookie";
import { DEFAULT_LIMIT, MAX_FILE_BYTES, NULL_BYTE } from "../consts/defaults";
import {
  FILE_NOT_FOUND,
  FILE_NOT_TEXT,
  FILE_SIZE_EXCEEDS,
  INVALID_FORM_DATA,
  INVALID_JSON_BODY,
  INVALID_MULTIPART,
  makeError,
  ONLY_TEXT_PLAIN
} from "../consts/errors";

export const paramsRoutes = new Hono();

paramsRoutes.get("/search", (c) => {
  const q = c.req.query("q")?.trim() || "none";

  const limitStr = c.req.query("limit");
  const limitNum = Number(limitStr);
  const limit = Number.isSafeInteger(limitNum) ? limitNum : DEFAULT_LIMIT;

  return c.json({ search: q, limit });
});

paramsRoutes.get("/url/:dynamic", (c) => {
  const dynamic = c.req.param("dynamic");
  return c.json({ dynamic });
});

paramsRoutes.get("/header", (c) => {
  const header = c.req.header("X-Custom-Header")?.trim() || "none";
  return c.json({ header });
});

paramsRoutes.post("/body", async (c) => {
  let body: Record<string, unknown>;
  try {
    body = await c.req.json();
  } catch (err) {
    return c.json(makeError(INVALID_JSON_BODY, err), 400);
  }

  if (typeof body !== "object" || body === null || Array.isArray(body)) {
    return c.json(makeError(INVALID_JSON_BODY, "expected a JSON object"), 400);
  }

  return c.json({ body });
});

paramsRoutes.get("/cookie", (c) => {
  const cookie = getCookie(c, "foo")?.trim() || "none";
  setCookie(c, "bar", "12345", { maxAge: 10, httpOnly: true, path: "/" });
  return c.json({ cookie });
});

paramsRoutes.post("/form", async (c) => {
  const contentType = c.req.header("content-type")?.toLowerCase() ?? "";
  if (!contentType.startsWith("application/x-www-form-urlencoded") && !contentType.startsWith("multipart/form-data")) {
    return c.json(
      makeError(INVALID_FORM_DATA, "expected content-type: application/x-www-form-urlencoded or multipart/form-data"),
      400
    );
  }

  let form: Record<string, string | File>;
  try {
    form = await c.req.parseBody();
  } catch (err) {
    return c.json(makeError(INVALID_FORM_DATA, err), 400);
  }

  if (typeof form !== "object" || form === null || Array.isArray(form)) {
    return c.json(makeError(INVALID_FORM_DATA, "expected form fields"), 400);
  }

  const name = typeof form.name === "string" && form.name.trim() !== "" ? form.name.trim() : "none";

  const ageStr = typeof form.age === "string" && form.age.trim() !== "" ? form.age.trim() : "0";
  const ageNum = Number(ageStr);
  const age = Number.isSafeInteger(ageNum) ? ageNum : 0;

  return c.json({ name, age });
});

paramsRoutes.post("/file", async (c) => {
  const contentType = c.req.header("content-type")?.toLowerCase() ?? "";
  if (!contentType.startsWith("multipart/form-data")) {
    return c.json(makeError(INVALID_MULTIPART, "expected content-type: multipart/form-data"), 400);
  }

  let form: FormData;
  try {
    form = await c.req.formData();
  } catch (err) {
    return c.json(makeError(INVALID_MULTIPART, err), 400);
  }

  const file = form.get("file");
  if (!(file instanceof File)) {
    return c.json(makeError(FILE_NOT_FOUND, "no file field named 'file' in form data"), 400);
  }
  if (!file.type || !file.type.startsWith("text/plain")) {
    return c.json(makeError(ONLY_TEXT_PLAIN, `received mimetype: ${file.type || "unknown"}`), 415);
  }
  if (file.size > MAX_FILE_BYTES) {
    return c.json(makeError(FILE_SIZE_EXCEEDS, `file size ${file.size} exceeds limit ${MAX_FILE_BYTES}`), 413);
  }

  const buffer = await file.arrayBuffer();
  const data = new Uint8Array(buffer);
  if (data.length > MAX_FILE_BYTES) {
    return c.json(makeError(FILE_SIZE_EXCEEDS, `file size ${data.length} exceeds limit ${MAX_FILE_BYTES}`), 413);
  }

  if (data.includes(NULL_BYTE)) {
    return c.json(makeError(FILE_NOT_TEXT, "file contains null bytes"), 415);
  }

  let content: string;
  try {
    content = new TextDecoder("utf-8", { fatal: true }).decode(data);
  } catch {
    return c.json(makeError(FILE_NOT_TEXT, "file is not valid UTF-8"), 415);
  }

  return c.json({
    filename: file.name,
    size: data.length,
    content
  });
});
