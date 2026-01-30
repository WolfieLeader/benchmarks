import type { FastifyInstance, FastifyRequest } from "fastify";
import { collectFormFields } from "../app";
import { DEFAULT_LIMIT, MAX_FILE_BYTES, NULL_BYTE, SNIFF_LEN } from "../consts/defaults";
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

export async function paramsRoutes(app: FastifyInstance) {
  app.get<{ Querystring: { q?: string; limit?: string } }>("/search", async (request) => {
    const q = request.query.q?.trim() || "none";

    const limitStr = request.query.limit?.trim();
    const limitNum = limitStr ? Number(limitStr) : Number.NaN;
    const limit = !limitStr?.includes(".") && Number.isSafeInteger(limitNum) ? limitNum : DEFAULT_LIMIT;

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
      return makeError(
        INVALID_FORM_DATA,
        "expected content-type: application/x-www-form-urlencoded or multipart/form-data"
      );
    }

    const fields = await collectFormFields(request);
    const name = typeof fields.name === "string" && fields.name.trim() !== "" ? fields.name.trim() : "none";

    const ageStr = typeof fields.age === "string" ? fields.age : "0";
    const ageNum = Number(ageStr);
    const age = !ageStr.includes(".") && Number.isSafeInteger(ageNum) ? ageNum : 0;

    return { name, age };
  });

  app.post("/file", async (request: FastifyRequest, reply) => {
    const contentType = request.headers["content-type"]?.toLowerCase() ?? "";
    if (!contentType.startsWith("multipart/form-data")) {
      reply.code(400);
      return makeError(INVALID_MULTIPART, "expected content-type: multipart/form-data");
    }

    const file = await request.file();
    if (!file) {
      reply.code(400);
      return makeError(FILE_NOT_FOUND, "no file field named 'file' in form data");
    }

    if (!file.mimetype || !file.mimetype.startsWith("text/plain")) {
      reply.code(415);
      return makeError(ONLY_TEXT_PLAIN, `received mimetype: ${file.mimetype || "unknown"}`);
    }

    const buffer = await file.toBuffer();
    if (buffer.length > MAX_FILE_BYTES || file.file.truncated) {
      reply.code(413);
      return makeError(FILE_SIZE_EXCEEDS, `file size ${buffer.length} exceeds limit ${MAX_FILE_BYTES}`);
    }

    const head = buffer.subarray(0, SNIFF_LEN);
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
