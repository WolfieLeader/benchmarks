import type { FastifyInstance, FastifyReply, FastifyRequest } from "fastify";
import type { MultipartFile } from "@fastify/multipart";

import {
  collectFormFields,
  MAX_FILE_BYTES,
  NULL_BYTE,
  SNIFF_LEN,
  toBuffer,
} from "../app";

type SearchQuery = {
  q?: string;
  limit?: string;
};

export async function paramsRoutes(app: FastifyInstance) {
  app.get<{ Querystring: SearchQuery }>("/search", async (request, reply) => {
    const q = request.query.q?.trim() || "none";

    const limitStr = request.query.limit ?? "";
    const limitNum = Number(limitStr);
    const limit = !limitStr.includes(".") && Number.isSafeInteger(limitNum) ? limitNum : 10;

    reply.send({ search: q, limit });
  });

  app.get<{ Params: { dynamic: string } }>("/url/:dynamic", async (request, reply) => {
    reply.send({ dynamic: request.params.dynamic });
  });

  app.get("/header", async (request, reply) => {
    const header = request.headers["x-custom-header"];
    reply.send({ header: typeof header === "string" ? header.trim() || "none" : "none" });
  });

  app.post("/body", async (request, reply) => {
    const body = request.body;

    if (typeof body !== "object" || body === null || Array.isArray(body)) {
      reply.code(400).send({ error: "invalid JSON body" });
      return;
    }

    reply.send({ body });
  });

  app.get("/cookie", async (request, reply) => {
    const cookie = request.cookies?.foo?.trim() || "none";
    reply.setCookie("bar", "12345", { maxAge: 10, httpOnly: true, path: "/" });
    reply.send({ cookie });
  });

  app.post("/form", async (request, reply) => {
    const contentType = request.headers["content-type"]?.toLowerCase() ?? "";
    if (
      !contentType.startsWith("application/x-www-form-urlencoded") &&
      !contentType.startsWith("multipart/form-data")
    ) {
      reply.code(400).send({ error: "invalid form data" });
      return;
    }

    const fields = await collectFormFields(request);
    const name = typeof fields.name === "string" && fields.name.trim() !== "" ? fields.name : "none";

    const ageStr = typeof fields.age === "string" ? fields.age : "0";
    const ageNum = Number(ageStr);
    const age = !ageStr.includes(".") && Number.isSafeInteger(ageNum) ? ageNum : 0;

    reply.send({ name, age });
  });

  app.post("/file", async (request: FastifyRequest, reply: FastifyReply) => {
    const contentType = request.headers["content-type"]?.toLowerCase() ?? "";
    if (!contentType.startsWith("multipart/form-data")) {
      reply.code(400).send({ error: "invalid multipart form data" });
      return;
    }

    const file = await request.file();
    if (!file) {
      reply.code(400).send({ error: "file not found in form data" });
      return;
    }

    if (!file.mimetype || !file.mimetype.startsWith("text/plain")) {
      reply.code(415).send({ error: "only text/plain files are allowed" });
      return;
    }

    const buffer = await toBuffer(file as MultipartFile);
    if (buffer.length > MAX_FILE_BYTES || file.file.truncated) {
      reply.code(413).send({ error: "file size exceeds limit" });
      return;
    }

    const head = buffer.subarray(0, SNIFF_LEN);
    if (head.includes(NULL_BYTE)) {
      reply.code(415).send({ error: "file does not look like plain text" });
      return;
    }

    if (buffer.includes(NULL_BYTE)) {
      reply.code(415).send({ error: "file does not look like plain text" });
      return;
    }

    let content: string;
    try {
      const decoder = new TextDecoder("utf-8", { fatal: true });
      content = decoder.decode(buffer);
    } catch {
      reply.code(415).send({ error: "file does not look like plain text" });
      return;
    }

    reply.send({
      filename: file.filename,
      size: buffer.length,
      content,
    });
  });
}
