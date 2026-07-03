import type { FastifyInstance, FastifyRequest } from "fastify";
import { collectFormFields } from "../app.js";
import { DEFAULT_LIMIT, MAX_FILE_BYTES, NULL_BYTE, SNIFF_LEN } from "../consts/defaults.js";
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
} from "../consts/errors.js";

// @fastify/multipart (via busboy) defaults a file part with no declared
// Content-Type to "text/plain" per RFC 2046, erasing the distinction the
// contract draws between an explicitly-declared text/plain part and an
// undeclared one. Read the declared type back from the raw wire bytes instead.
function declaredFileContentType(rawBody: Buffer, requestContentType: string): string | null {
  const boundaryMatch = /boundary=(?:"([^"]+)"|([^;]+))/i.exec(requestContentType);
  const boundary = (boundaryMatch?.[1] ?? boundaryMatch?.[2])?.trim();
  if (!boundary) return null;

  const raw = rawBody.toString("latin1");
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

function looksLikeText(bytes: Buffer): boolean {
  if (bytes.includes(NULL_BYTE)) return false;
  try {
    new TextDecoder("utf-8", { fatal: true }).decode(bytes);
    return true;
  } catch {
    return false;
  }
}

export async function paramsRoutes(app: FastifyInstance) {
  app.get<{ Querystring: { q?: string; limit?: string } }>("/search", async (request) => {
    const q = request.query.q?.trim() || "none";

    const limitStr = request.query.limit;
    const limitNum = Number(limitStr);
    const limit = Number.isSafeInteger(limitNum) && !limitStr?.includes(".") ? limitNum : DEFAULT_LIMIT;

    return { search: q, limit };
  });

  app.get<{ Params: { dynamic: string } }>("/url/:dynamic", async (request) => {
    return { dynamic: request.params.dynamic };
  });

  app.get("/header", async (request) => {
    const header = request.headers["x-custom-header"];
    return {
      header: typeof header === "string" ? header.trim() || "none" : "none"
    };
  });

  app.post("/body", async (request, reply) => {
    const body = request.body;

    if (typeof body !== "object" || body === null || Array.isArray(body)) {
      reply.code(400);
      return makeError(INVALID_JSON_BODY, "expected a JSON object");
    }

    return { body };
  });

  app.get("/cookie", async (request, reply) => {
    const cookie = request.cookies?.foo?.trim() || "none";
    reply.setCookie("bar", "12345", { maxAge: 10, httpOnly: true, path: "/" });
    return { cookie };
  });

  app.post("/form", async (request, reply) => {
    const contentType = request.headers["content-type"]?.toLowerCase() ?? "";
    if (
      !contentType.startsWith("application/x-www-form-urlencoded") &&
      !contentType.startsWith("multipart/form-data")
    ) {
      reply.code(400);
      return makeError(INVALID_FORM_DATA, EXPECTED_FORM_CONTENT_TYPE);
    }

    const fields = await collectFormFields(request);
    const name = fields.name?.trim() || "none";

    const ageStr = fields.age?.trim() || "0";
    const ageNum = Number(ageStr);
    const age = Number.isSafeInteger(ageNum) ? ageNum : 0;

    return { name, age };
  });

  app.post("/file", async (request: FastifyRequest, reply) => {
    const contentType = request.headers["content-type"]?.toLowerCase() ?? "";
    if (!contentType.startsWith("multipart/form-data")) {
      reply.code(400);
      return makeError(INVALID_MULTIPART, EXPECTED_MULTIPART_CONTENT_TYPE);
    }

    // Tee the raw request body so the file part's declared Content-Type can be
    // read from the wire (busboy hides its absence — see above). Listeners must
    // attach before `request.file()` starts consuming the stream.
    const rawChunks: Buffer[] = [];
    request.raw.on("data", (chunk: Buffer) => rawChunks.push(chunk));
    const rawDone = new Promise<void>((res) => request.raw.on("end", () => res()));

    const file = await request.file();
    if (!file) {
      reply.code(400);
      return makeError(FILE_NOT_FOUND, "no file field named 'file' in form data");
    }

    const buffer = await file.toBuffer();
    await rawDone;
    const rawBody = Buffer.concat(rawChunks);

    const head = buffer.subarray(0, SNIFF_LEN);
    const declared = declaredFileContentType(rawBody, request.headers["content-type"] ?? "");
    if (declared !== null) {
      // Trust the declared type for the allow-decision; a lie is caught by the
      // content inspection below (-> FILE_NOT_TEXT).
      if (!declared.toLowerCase().startsWith("text/plain")) {
        reply.code(415);
        return makeError(ONLY_TEXT_PLAIN, `received mimetype: ${declared}`);
      }
    } else if (!looksLikeText(head)) {
      // No declared type: sniff the bytes; non-text is rejected as a type error.
      reply.code(415);
      return makeError(ONLY_TEXT_PLAIN, "received mimetype: unknown");
    }

    if (buffer.length > MAX_FILE_BYTES || file.file.truncated) {
      reply.code(413);
      return makeError(FILE_SIZE_EXCEEDS, `file size ${buffer.length} exceeds limit ${MAX_FILE_BYTES}`);
    }
    if (head.includes(NULL_BYTE)) {
      reply.code(415);
      return makeError(FILE_NOT_TEXT, "file contains null bytes in header");
    }

    if (buffer.includes(NULL_BYTE)) {
      reply.code(415);
      return makeError(FILE_NOT_TEXT, "file contains null bytes");
    }

    let content: string;
    try {
      const decoder = new TextDecoder("utf-8", { fatal: true });
      content = decoder.decode(buffer);
    } catch {
      reply.code(415);
      return makeError(FILE_NOT_TEXT, "file is not valid UTF-8");
    }

    return {
      filename: file.filename,
      size: buffer.length,
      content
    };
  });
}
