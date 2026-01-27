import { Hono } from "hono";
import { getCookie, setCookie } from "hono/cookie";
import { MAX_FILE_BYTES, NULL_BYTE, SNIFF_LEN, DEFAULT_LIMIT } from "../consts/defaults";
import {
  INVALID_JSON_BODY,
  INVALID_FORM_DATA,
  INVALID_MULTIPART,
  FILE_NOT_FOUND,
  FILE_SIZE_EXCEEDS,
  ONLY_TEXT_PLAIN,
  FILE_NOT_TEXT
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
  } catch {
    return c.json({ error: INVALID_JSON_BODY }, 400);
  }

  if (typeof body !== "object" || body === null || Array.isArray(body)) {
    return c.json({ error: INVALID_JSON_BODY }, 400);
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
    return c.json({ error: INVALID_FORM_DATA }, 400);
  }

  let form: Record<string, string | File>;
  try {
    form = await c.req.parseBody();
  } catch {
    return c.json({ error: INVALID_FORM_DATA }, 400);
  }

  if (typeof form !== "object" || form === null || Array.isArray(form)) {
    return c.json({ error: INVALID_FORM_DATA }, 400);
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
    return c.json({ error: INVALID_MULTIPART }, 400);
  }

  let form: FormData;
  try {
    form = await c.req.formData();
  } catch {
    return c.json({ error: INVALID_MULTIPART }, 400);
  }

  const file = form.get("file");
  if (!(file instanceof File)) {
    return c.json({ error: FILE_NOT_FOUND }, 400);
  }
  if (!file.type || !file.type.startsWith("text/plain")) {
    return c.json({ error: ONLY_TEXT_PLAIN }, 415);
  }
  if (file.size > MAX_FILE_BYTES) {
    return c.json({ error: FILE_SIZE_EXCEEDS }, 413);
  }

  const buffer = await file.arrayBuffer();
  const data = new Uint8Array(buffer);
  if (data.length > MAX_FILE_BYTES) {
    return c.json({ error: FILE_SIZE_EXCEEDS }, 413);
  }

  const head = data.slice(0, SNIFF_LEN);
  if (head.includes(NULL_BYTE)) {
    return c.json({ error: FILE_NOT_TEXT }, 415);
  }

  if (data.includes(NULL_BYTE)) {
    return c.json({ error: FILE_NOT_TEXT }, 415);
  }

  let content: string;
  try {
    content = new TextDecoder("utf-8", { fatal: true }).decode(data);
  } catch {
    return c.json({ error: FILE_NOT_TEXT }, 415);
  }

  return c.json({
    filename: file.name,
    size: data.length,
    content
  });
});
