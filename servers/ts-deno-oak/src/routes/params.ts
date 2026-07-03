import { Router } from "@oak/oak";
import { DEFAULT_LIMIT, MAX_FILE_BYTES, NULL_BYTE, SNIFF_LEN } from "../consts/defaults.ts";
import {
  EXPECTED_FORM_CONTENT_TYPE,
  EXPECTED_MULTIPART_CONTENT_TYPE,
  FILE_NOT_FOUND,
  FILE_NOT_TEXT,
  FILE_SIZE_EXCEEDS,
  INVALID_FORM_DATA,
  INVALID_JSON_BODY,
  INVALID_MULTIPART,
  makeError,
  ONLY_TEXT_PLAIN
} from "../consts/errors.ts";

export const paramsRoutes = new Router();

paramsRoutes.get("/search", (ctx) => {
  const q = ctx.request.url.searchParams.get("q")?.trim() || "none";
  const limitStr = ctx.request.url.searchParams.get("limit")?.trim();
  const limitNum = limitStr ? Number(limitStr) : Number.NaN;
  const limit = Number.isSafeInteger(limitNum) && !limitStr?.includes(".") ? limitNum : DEFAULT_LIMIT;
  ctx.response.body = { search: q, limit };
});

paramsRoutes.get("/url/:dynamic", (ctx) => {
  const dynamic = ctx.params.dynamic;
  ctx.response.body = { dynamic };
});

paramsRoutes.get("/header", (ctx) => {
  const header = ctx.request.headers.get("X-Custom-Header")?.trim() || "none";
  ctx.response.body = { header };
});

paramsRoutes.post("/body", async (ctx) => {
  if (!ctx.request.hasBody) {
    ctx.response.status = 400;
    ctx.response.body = makeError(INVALID_JSON_BODY, "No body");
    return;
  }

  let body: unknown;
  try {
    body = await ctx.request.body.json();
  } catch (err) {
    ctx.response.status = 400;
    ctx.response.body = makeError(INVALID_JSON_BODY, err);
    return;
  }

  if (typeof body !== "object" || body === null || Array.isArray(body)) {
    ctx.response.status = 400;
    ctx.response.body = makeError(INVALID_JSON_BODY, "expected a JSON object");
    return;
  }

  ctx.response.body = { body };
});

paramsRoutes.get("/cookie", async (ctx) => {
  const cookie = (await ctx.cookies.get("foo"))?.trim() || "none";

  await ctx.cookies.set("bar", "12345", {
    maxAge: 10,
    httpOnly: true,
    path: "/"
  });
  ctx.response.body = { cookie };
});

paramsRoutes.post("/form", async (ctx) => {
  const contentType = ctx.request.headers.get("content-type")?.toLowerCase() ?? "";
  if (!contentType.startsWith("application/x-www-form-urlencoded") && !contentType.startsWith("multipart/form-data")) {
    ctx.response.status = 400;
    ctx.response.body = makeError(INVALID_FORM_DATA, EXPECTED_FORM_CONTENT_TYPE);
    return;
  }

  const form: Record<string, string> = {};
  try {
    const formData = await ctx.request.body.formData();
    formData.forEach((value, key) => {
      if (typeof value === "string") {
        form[key] = value;
      }
    });
  } catch (err) {
    ctx.response.status = 400;
    ctx.response.body = makeError(INVALID_FORM_DATA, err);
    return;
  }

  const name = form.name?.trim() || "none";

  const ageStr = form.age?.trim() || "0";
  const ageNum = Number(ageStr);
  const age = Number.isSafeInteger(ageNum) ? ageNum : 0;

  ctx.response.body = { name, age };
});

paramsRoutes.post("/file", async (ctx) => {
  const contentType = ctx.request.headers.get("content-type")?.toLowerCase() ?? "";
  if (!contentType.startsWith("multipart/form-data")) {
    ctx.response.status = 400;
    ctx.response.body = makeError(INVALID_MULTIPART, EXPECTED_MULTIPART_CONTENT_TYPE);
    return;
  }

  let formData: FormData;
  try {
    formData = await ctx.request.body.formData();
  } catch (err) {
    ctx.response.status = 400;
    ctx.response.body = makeError(INVALID_MULTIPART, err);
    return;
  }

  const file = formData.get("file");
  if (!(file instanceof File)) {
    ctx.response.status = 400;
    ctx.response.body = makeError(FILE_NOT_FOUND, "no file field named 'file' in form data");
    return;
  }

  if (!file.type || !file.type.startsWith("text/plain")) {
    ctx.response.status = 415;
    ctx.response.body = makeError(ONLY_TEXT_PLAIN, `received mimetype: ${file.type || "unknown"}`);
    return;
  }

  if (file.size > MAX_FILE_BYTES) {
    ctx.response.status = 413;
    ctx.response.body = makeError(FILE_SIZE_EXCEEDS, `file size ${file.size} exceeds limit ${MAX_FILE_BYTES}`);
    return;
  }

  const data = new Uint8Array(await file.arrayBuffer());

  if (data.length > MAX_FILE_BYTES) {
    ctx.response.status = 413;
    ctx.response.body = makeError(FILE_SIZE_EXCEEDS, `file size ${data.length} exceeds limit ${MAX_FILE_BYTES}`);
    return;
  }

  const head = data.slice(0, SNIFF_LEN);
  if (head.includes(NULL_BYTE)) {
    ctx.response.status = 415;
    ctx.response.body = makeError(FILE_NOT_TEXT, "file contains null bytes in header");
    return;
  }

  if (data.includes(NULL_BYTE)) {
    ctx.response.status = 415;
    ctx.response.body = makeError(FILE_NOT_TEXT, "file contains null bytes");
    return;
  }

  let content: string;
  try {
    content = new TextDecoder("utf-8", { fatal: true }).decode(data);
  } catch {
    ctx.response.status = 415;
    ctx.response.body = makeError(FILE_NOT_TEXT, "file is not valid UTF-8");
    return;
  }

  ctx.response.body = {
    filename: file.name,
    size: data.length,
    content
  };
});
