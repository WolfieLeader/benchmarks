import type { FastifyInstance, FastifyRequest } from "fastify";
import type { MultipartFile } from "@fastify/multipart";
import { MAX_FILE_BYTES, NULL_BYTE, SNIFF_LEN, DEFAULT_LIMIT } from "../consts/defaults";
import {
  INVALID_JSON_BODY,
  INVALID_FORM_DATA,
  INVALID_MULTIPART,
  FILE_NOT_FOUND,
  FILE_SIZE_EXCEEDS,
  ONLY_TEXT_PLAIN,
  FILE_NOT_TEXT,
} from "../consts/errors";
import { collectFormFields, toBuffer } from "../app";

export async function paramsRoutes(app: FastifyInstance) {
  app.get<{ Querystring: { q?: string; limit?: string } }>("/search", async (request) => {
    const q = request.query.q?.trim() || "none";

    const limitStr = request.query.limit?.trim();
    const limitNum = limitStr ? Number(limitStr) : NaN;
    const limit = !limitStr?.includes(".") && Number.isSafeInteger(limitNum) ? limitNum : DEFAULT_LIMIT;

    return { search: q, limit };
  });

  app.get<{ Params: { dynamic: string } }>("/url/:dynamic", async (request) => {
    return { dynamic: request.params.dynamic };
  });

  app.get("/header", async (request) => {
    const header = request.headers["x-custom-header"];
    return { header: typeof header === "string" ? header.trim() || "none" : "none" };
  });

  app.post("/body", async (request, reply) => {
    const body = request.body;

    if (typeof body !== "object" || body === null || Array.isArray(body)) {
      reply.code(400);
      return { error: INVALID_JSON_BODY };
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
      return { error: INVALID_FORM_DATA };
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
      return { error: INVALID_MULTIPART };
    }

    const file = await request.file();
    if (!file) {
      reply.code(400);
      return { error: FILE_NOT_FOUND };
    }

    if (!file.mimetype || !file.mimetype.startsWith("text/plain")) {
      reply.code(415);
      return { error: ONLY_TEXT_PLAIN };
    }

    const buffer = await toBuffer(file as MultipartFile);
    if (buffer.length > MAX_FILE_BYTES || file.file.truncated) {
      reply.code(413);
      return { error: FILE_SIZE_EXCEEDS };
    }

    const head = buffer.subarray(0, SNIFF_LEN);
    if (head.includes(NULL_BYTE)) {
      reply.code(415);
      return { error: FILE_NOT_TEXT };
    }

    if (buffer.includes(NULL_BYTE)) {
      reply.code(415);
      return { error: FILE_NOT_TEXT };
    }

    let content: string;
    try {
      const decoder = new TextDecoder("utf-8", { fatal: true });
      content = decoder.decode(buffer);
    } catch {
      reply.code(415);
      return { error: FILE_NOT_TEXT };
    }

    return {
      filename: file.filename,
      size: buffer.length,
      content,
    };
  });
}
