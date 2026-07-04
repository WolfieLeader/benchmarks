// /params routes: query/path/header/cookie/body/form/file echo + validation.
// The multipart file handler reparses the raw body to recover the file part's
// DECLARED Content-Type off the wire before the runtime's formData() normalizes
// it away — load-bearing for identical classification across Node/Bun/Deno.

import {
  DEFAULT_LIMIT,
  EXPECTED_FORM_CONTENT_TYPE,
  EXPECTED_MULTIPART_CONTENT_TYPE,
  FILE_NOT_FOUND,
  FILE_NOT_TEXT,
  FILE_SIZE_EXCEEDS,
  INVALID_FORM_DATA,
  INVALID_JSON_BODY,
  INVALID_MULTIPART,
  makeError,
  MAX_FILE_BYTES,
  NULL_BYTE,
  ONLY_TEXT_PLAIN,
  SNIFF_LEN
} from "@bench/shared";
import { Hono } from "hono";
import { getCookie, setCookie } from "hono/cookie";

// Recover the declared Content-Type of the file part (the part with a filename)
// straight off the multipart wire, or null when that part carried no
// Content-Type header at all. This is load-bearing across runtimes: each
// runtime's `Request.formData()` normalizes an UNDECLARED file part's type
// differently (Node/undici -> "text/plain", Bun -> "", Deno ->
// "application/octet-stream"), erasing the declared-vs-undeclared distinction the
// contract draws between `file_anti_sniffing_binary_lying_as_text` (declared
// text/plain, expects FILE_NOT_TEXT) and `file_rejects_sniffed_binary_without_
// declared_type` (undeclared, expects ONLY_TEXT_PLAIN). Reading the raw header
// off the wire is the same canon ts-express uses (servers/ts-express params.ts).
function declaredFileContentType(bytes: Uint8Array, requestContentType: string): string | null {
  const boundaryMatch = /boundary=(?:"([^"]+)"|([^;]+))/i.exec(requestContentType);
  const boundary = (boundaryMatch?.[1] ?? boundaryMatch?.[2])?.trim();
  if (!boundary) return null;

  const raw = new TextDecoder("latin1").decode(bytes);
  for (const segment of raw.split(`--${boundary}`)) {
    const headerEnd = segment.indexOf("\r\n\r\n");
    if (headerEnd === -1) continue;
    const headers = segment.slice(0, headerEnd);
    if (!/content-disposition:[^\r\n]*\bfilename=/i.test(headers)) continue;
    const typeMatch = /\r\ncontent-type:\s*([^\r\n]+)/i.exec(`\r\n${headers}`);
    return typeMatch?.[1]?.trim() ?? null;
  }
  return null;
}

function looksLikeText(bytes: Uint8Array): boolean {
  if (bytes.includes(NULL_BYTE)) return false;
  try {
    new TextDecoder("utf-8", { fatal: true }).decode(bytes);
    return true;
  } catch {
    return false;
  }
}

export function createParamsRoutes(): Hono {
  const paramsRoutes = new Hono();

  paramsRoutes.get("/search", (c) => {
    const q = c.req.query("q")?.trim() || "none";

    const limitStr = c.req.query("limit");
    const limitNum = Number(limitStr);
    const limit = Number.isSafeInteger(limitNum) && !limitStr?.includes(".") ? limitNum : DEFAULT_LIMIT;

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
    if (
      !contentType.startsWith("application/x-www-form-urlencoded") &&
      !contentType.startsWith("multipart/form-data")
    ) {
      return c.json(makeError(INVALID_FORM_DATA, EXPECTED_FORM_CONTENT_TYPE), 400);
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

    const name = (typeof form.name === "string" && form.name.trim()) || "none";

    const ageStr = (typeof form.age === "string" && form.age.trim()) || "0";
    const ageNum = Number(ageStr);
    const age = Number.isSafeInteger(ageNum) ? ageNum : 0;

    return c.json({ name, age });
  });

  paramsRoutes.post("/file", async (c) => {
    const contentType = c.req.header("content-type")?.toLowerCase() ?? "";
    if (!contentType.startsWith("multipart/form-data")) {
      return c.json(makeError(INVALID_MULTIPART, EXPECTED_MULTIPART_CONTENT_TYPE), 400);
    }

    // Read the raw body once, then reparse the same bytes for the File: this
    // recovers the file part's declared Content-Type off the wire (see
    // declaredFileContentType) before the runtime's formData() can normalize it
    // away, so all three runtimes classify identically.
    const ctHeader = c.req.header("content-type") ?? "";
    let raw: Uint8Array;
    try {
      raw = new Uint8Array(await c.req.arrayBuffer());
    } catch (err) {
      return c.json(makeError(INVALID_MULTIPART, err), 400);
    }

    let form: FormData;
    try {
      form = await new Request("http://local/", {
        method: "POST",
        headers: { "content-type": ctHeader },
        body: raw
      }).formData();
    } catch (err) {
      return c.json(makeError(INVALID_MULTIPART, err), 400);
    }

    const file = form.get("file");
    if (!(file instanceof File)) {
      return c.json(makeError(FILE_NOT_FOUND, "no file field named 'file' in form data"), 400);
    }

    const data = new Uint8Array(await file.arrayBuffer());
    const head = data.slice(0, SNIFF_LEN);

    // Classify by the declared type when the part declared one; otherwise sniff
    // the bytes and reject non-text as a type error (mirrors ts-express).
    const declared = declaredFileContentType(raw, ctHeader);
    if (declared !== null) {
      if (!declared.toLowerCase().startsWith("text/plain")) {
        return c.json(makeError(ONLY_TEXT_PLAIN, `received mimetype: ${declared}`), 415);
      }
    } else if (!looksLikeText(head)) {
      return c.json(makeError(ONLY_TEXT_PLAIN, "received mimetype: unknown"), 415);
    }

    if (data.length > MAX_FILE_BYTES) {
      return c.json(makeError(FILE_SIZE_EXCEEDS, `file size ${data.length} exceeds limit ${MAX_FILE_BYTES}`), 413);
    }

    if (head.includes(NULL_BYTE)) {
      return c.json(makeError(FILE_NOT_TEXT, "file contains null bytes in header"), 415);
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

  return paramsRoutes;
}
