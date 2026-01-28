import { Elysia } from "elysia";
import { DEFAULT_LIMIT, MAX_FILE_BYTES, NULL_BYTE, SNIFF_LEN } from "../consts/defaults";
import {
  FILE_NOT_FOUND,
  FILE_NOT_TEXT,
  FILE_SIZE_EXCEEDS,
  INVALID_FORM_DATA,
  INVALID_JSON_BODY,
  INVALID_MULTIPART,
  ONLY_TEXT_PLAIN
} from "../consts/errors";

export const paramsRouter = new Elysia();

paramsRouter.get("/search", ({ query }) => {
  const q = query.q?.trim() || "none";
  const limitStr = query.limit;
  const limitNum = Number(limitStr);
  const limit = Number.isSafeInteger(limitNum) ? limitNum : DEFAULT_LIMIT;
  return { search: q, limit };
});

paramsRouter.get("/url/:dynamic", ({ params: { dynamic } }) => {
  return { dynamic };
});

paramsRouter.get("/header", ({ headers }) => {
  const header = headers["x-custom-header"]?.trim() || "none";
  return { header };
});

paramsRouter.post("/body", ({ body, set }) => {
  if (typeof body !== "object" || body === null || Array.isArray(body)) {
    set.status = 400;
    return { error: INVALID_JSON_BODY };
  }
  return { body };
});

paramsRouter.get("/cookie", ({ cookie }) => {
  const cookieVal = cookie.foo?.value?.trim() || "none";

  cookie.bar.value = "12345";
  cookie.bar.maxAge = 10;
  cookie.bar.httpOnly = true;
  cookie.bar.path = "/";

  return { cookie: cookieVal };
});

paramsRouter.post("/form", ({ body, set, headers }) => {
  const contentType = headers["content-type"]?.toLowerCase() ?? "";
  if (!contentType.startsWith("application/x-www-form-urlencoded") && !contentType.startsWith("multipart/form-data")) {
    set.status = 400;
    return { error: INVALID_FORM_DATA };
  }

  if (typeof body !== "object" || body === null || Array.isArray(body)) {
    set.status = 400;
    return { error: INVALID_FORM_DATA };
  }

  const form = body as Record<string, unknown>;

  const name = typeof form.name === "string" && form.name.trim() !== "" ? form.name.trim() : "none";

  const ageStr = typeof form.age === "string" && form.age.trim() !== "" ? form.age.trim() : "0";
  const ageNum = Number(ageStr);
  const age = Number.isSafeInteger(ageNum) ? ageNum : 0;

  return { name, age };
});

paramsRouter.post("/file", async ({ body, set, headers }) => {
  const contentType = headers["content-type"]?.toLowerCase() ?? "";
  if (!contentType.startsWith("multipart/form-data")) {
    set.status = 400;
    return { error: INVALID_MULTIPART };
  }

  const form = body as Record<string, unknown>;
  const file = form.file;

  if (!(file instanceof File)) {
    set.status = 400;
    return { error: FILE_NOT_FOUND };
  }
  if (!file.type || !file.type.startsWith("text/plain")) {
    set.status = 415;
    return { error: ONLY_TEXT_PLAIN };
  }
  if (file.size > MAX_FILE_BYTES) {
    set.status = 413;
    return { error: FILE_SIZE_EXCEEDS };
  }

  const buffer = await file.arrayBuffer();
  const data = new Uint8Array(buffer);
  if (data.length > MAX_FILE_BYTES) {
    set.status = 413;
    return { error: FILE_SIZE_EXCEEDS };
  }

  const head = data.slice(0, SNIFF_LEN);
  if (head.includes(NULL_BYTE)) {
    set.status = 415;
    return { error: FILE_NOT_TEXT };
  }

  if (data.includes(NULL_BYTE)) {
    set.status = 415;
    return { error: FILE_NOT_TEXT };
  }

  let content: string;
  try {
    content = new TextDecoder("utf-8", { fatal: true }).decode(data);
  } catch {
    set.status = 415;
    return { error: FILE_NOT_TEXT };
  }

  return {
    filename: file.name,
    size: data.length,
    content
  };
});
