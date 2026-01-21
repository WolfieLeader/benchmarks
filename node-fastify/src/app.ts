import fastify, { type FastifyInstance, type FastifyRequest } from "fastify";
import cookie from "@fastify/cookie";
import formbody from "@fastify/formbody";
import multipart, { type MultipartFile } from "@fastify/multipart";
import { paramsRoutes } from "./routes/params";
import { env } from "./config/env";
import { NOT_FOUND, INTERNAL_ERROR } from "./consts/errors";
import { MAX_FILE_BYTES } from "./consts/defaults";

export type FormFields = Record<string, string>;

export async function createApp(): Promise<FastifyInstance> {
  const app = fastify({ logger: false });

  await app.register(cookie);
  await app.register(formbody);
  await app.register(multipart, { limits: { fileSize: MAX_FILE_BYTES } });

  if (env.ENV !== "prod") {
    app.addHook("onRequest", async (request) => {
      (request as any).startTime = Date.now();
      console.log(`<-- ${request.method} ${request.url}`);
    });

    app.addHook("onResponse", async (request, reply) => {
      const start = (request as any).startTime ?? Date.now();
      const ms = Date.now() - start;
      console.log(`--> ${request.method} ${request.url} ${reply.statusCode} ${ms}ms`);
    });
  }

  app.get("/", async () => ({ message: "Hello, World!" }));
  app.get("/health", async () => "OK");

  await app.register(paramsRoutes, { prefix: "/params" });

  app.setNotFoundHandler(async (_req, reply) => {
    reply.code(404);
    return { error: NOT_FOUND };
  });

  app.setErrorHandler(async (error, _req, reply) => {
    reply.code(500);
    return { error: (error as Error).message || INTERNAL_ERROR };
  });

  return app;
}

export async function collectFormFields(request: FastifyRequest): Promise<FormFields> {
  if (request.isMultipart()) {
    const fields: FormFields = {};
    const parts = request.parts();
    for await (const part of parts) {
      if (part.type === "file") {
        part.file.resume();
        continue;
      }
      fields[part.fieldname] = part.value as string;
    }
    return fields;
  }

  const body = (request.body ?? {}) as Record<string, unknown>;
  const fields: FormFields = {};
  for (const [key, value] of Object.entries(body)) {
    if (typeof value === "string") {
      fields[key] = value;
    }
  }
  return fields;
}

export async function toBuffer(file: MultipartFile): Promise<Buffer> {
  const chunks: Buffer[] = [];
  for await (const chunk of file.file) {
    if (Buffer.isBuffer(chunk)) {
      chunks.push(chunk);
    } else if (typeof chunk === "string") {
      chunks.push(Buffer.from(chunk));
    }
  }
  return Buffer.concat(chunks);
}
