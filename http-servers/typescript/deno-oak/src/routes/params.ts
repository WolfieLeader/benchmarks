import { Router } from "@oak/oak";
import {
  DEFAULT_LIMIT,
  MAX_FILE_BYTES,
  NULL_BYTE,
  SNIFF_LEN,
} from "../consts/defaults.ts";
import {
  FILE_NOT_FOUND,
  FILE_NOT_TEXT,
  FILE_SIZE_EXCEEDS,
  INVALID_FORM_DATA,
  INVALID_JSON_BODY,
  INVALID_MULTIPART,
  ONLY_TEXT_PLAIN,
} from "../consts/errors.ts";

export const paramsRoutes = new Router();

paramsRoutes.get("/search", (ctx) => {
  const q = ctx.request.url.searchParams.get("q")?.trim() || "none";
  const limitStr = ctx.request.url.searchParams.get("limit")?.trim();
  const limitNum = limitStr ? Number(limitStr) : Number.NaN;
  const limit = Number.isSafeInteger(limitNum) ? limitNum : DEFAULT_LIMIT;
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
  let body: unknown;
  try {
    if (!ctx.request.hasBody) {
      throw new Error("No body");
    }
    body = await ctx.request.body.json();
  } catch {
    ctx.response.status = 400;
    ctx.response.body = { error: INVALID_JSON_BODY };
    return;
  }

  if (typeof body !== "object" || body === null || Array.isArray(body)) {
    ctx.response.status = 400;
    ctx.response.body = { error: INVALID_JSON_BODY };
    return;
  }

  ctx.response.body = { body };
});

paramsRoutes.get("/cookie", async (ctx) => {
  const cookieVal = await ctx.cookies.get("foo");
  const cookie = cookieVal?.trim() || "none";

  await ctx.cookies.set("bar", "12345", {
    maxAge: 10,
    httpOnly: true,
    path: "/",
  });
  ctx.response.body = { cookie };
});

paramsRoutes.post("/form", async (ctx) => {
  const contentType = ctx.request.headers.get("content-type")?.toLowerCase() ??
    "";
  if (
    !contentType.startsWith("application/x-www-form-urlencoded") &&
    !contentType.startsWith("multipart/form-data")
  ) {
    ctx.response.status = 400;
    ctx.response.body = { error: INVALID_FORM_DATA };
    return;
  }

  const form: Record<string, string> = {};
  try {
    const body = ctx.request.body;

    if (body.type() === "form") {
      const pairs = await body.form();
      for (const [key, value] of pairs) {
        form[key] = value;
      }
    } else if (body.type() === "form-data") {
      const result = await body.formData();
      result.forEach((value: FormDataEntryValue, key: string) => {
        if (typeof value === "string") {
          form[key] = value;
        }
      });
    } else {
      throw new Error("Invalid type");
    }
  } catch {
    ctx.response.status = 400;
    ctx.response.body = { error: INVALID_FORM_DATA };
    return;
  }

  const name = typeof form.name === "string" && form.name.trim() !== ""
    ? form.name.trim()
    : "none";

  const ageStr = typeof form.age === "string" && form.age.trim() !== ""
    ? form.age.trim()
    : "0";
  const ageNum = Number(ageStr);
  const age = Number.isSafeInteger(ageNum) ? ageNum : 0;

  ctx.response.body = { name, age };
});

paramsRoutes.post("/file", async (ctx) => {
  const contentType = ctx.request.headers.get("content-type")?.toLowerCase() ??
    "";
  if (!contentType.startsWith("multipart/form-data")) {
    ctx.response.status = 400;
    ctx.response.body = { error: INVALID_MULTIPART };
    return;
  }

  let file: File | null = null;
  try {
    const body = ctx.request.body;
    if (body.type() === "form-data") {
      const formData = await body.formData();
      const fileEntry = formData.get("file");
      if (fileEntry instanceof File) {
        file = fileEntry;
      }
    }
  } catch {
    ctx.response.status = 400;
    ctx.response.body = { error: INVALID_MULTIPART };
    return;
  }

  if (!file) {
    ctx.response.status = 400;
    ctx.response.body = { error: FILE_NOT_FOUND };
    return;
  }

  if (!file.type || !file.type.startsWith("text/plain")) {
    ctx.response.status = 415;
    ctx.response.body = { error: ONLY_TEXT_PLAIN };
    return;
  }

  if (file.size > MAX_FILE_BYTES) {
    ctx.response.status = 413;
    ctx.response.body = { error: FILE_SIZE_EXCEEDS };
    return;
  }

  const buffer = await new Response(file.stream()).arrayBuffer();
  const data = new Uint8Array(buffer);

  if (data.length > MAX_FILE_BYTES) {
    ctx.response.status = 413;
    ctx.response.body = { error: FILE_SIZE_EXCEEDS };
    return;
  }

  const head = data.slice(0, SNIFF_LEN);
  if (head.includes(NULL_BYTE)) {
    ctx.response.status = 415;
    ctx.response.body = { error: FILE_NOT_TEXT };
    return;
  }

  if (data.includes(NULL_BYTE)) {
    ctx.response.status = 415;
    ctx.response.body = { error: FILE_NOT_TEXT };
    return;
  }

  let content: string;
  try {
    content = new TextDecoder("utf-8", { fatal: true }).decode(data);
  } catch {
    ctx.response.status = 415;
    ctx.response.body = { error: FILE_NOT_TEXT };
    return;
  }

  ctx.response.body = {
    filename: file.name,
    size: data.length,
    content,
  };
});
