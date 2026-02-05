import cookie from "@fastify/cookie";
import formbody from "@fastify/formbody";
import multipart from "@fastify/multipart";
import fastify, { type FastifyInstance, type FastifyRequest } from "fastify";
import { env } from "./config/env";
import { MAX_FILE_BYTES, MAX_REQUEST_BYTES } from "./consts/defaults";
import {
  FILE_SIZE_EXCEEDS,
  INTERNAL_ERROR,
  INVALID_JSON_BODY,
  INVALID_MULTIPART,
  makeError,
  NOT_FOUND
} from "./consts/errors";
import { dbRoutes } from "./routes/db";
import { paramsRoutes } from "./routes/params";

export type FormFields = Record<string, string>;

export async function createApp(): Promise<FastifyInstance> {
  const app = fastify({ logger: false, bodyLimit: MAX_REQUEST_BYTES });

  await app.register(cookie);
  await app.register(formbody);
  await app.register(multipart, { limits: { fileSize: MAX_FILE_BYTES } });

  if (env.ENV !== "prod") {
    app.addHook("onRequest", async (request) => {
      request.startTime = Date.now();
      console.log(`<-- ${request.method} ${request.url}`);
    });

    app.addHook("onResponse", async (request, reply) => {
      const start = request.startTime ?? Date.now();
      const ms = Date.now() - start;
      console.log(`--> ${request.method} ${request.url} ${reply.statusCode} ${ms}ms`);
    });
  }

  app.get("/", async () => {
    return { hello: "world" };
  });
  app.get("/health", async (_req, reply) => {
    reply.type("text/plain").send("OK");
  });

  await app.register(paramsRoutes, { prefix: "/params" });
  await app.register(dbRoutes, { prefix: "/db" });

  app.setNotFoundHandler(async (_req, reply) => {
    reply.code(404);
    return { error: NOT_FOUND };
  });

  app.setErrorHandler(async (error, _req, reply) => {
    const err = error as { code?: string; statusCode?: number; message?: string };
    let statusCode = typeof err.statusCode === "number" ? err.statusCode : 500;
    let message = INTERNAL_ERROR;
    const details = err.message || undefined;

    if (err.code === "FST_ERR_CTP_INVALID_JSON_BODY" || err.code === "FST_ERR_CTP_EMPTY_JSON_BODY") {
      statusCode = 400;
      message = INVALID_JSON_BODY;
    } else if (err.code === "FST_ERR_CTP_BODY_TOO_LARGE") {
      statusCode = 413;
      message = FILE_SIZE_EXCEEDS;
    } else if (err.code === "FST_ERR_MULTIPART_LIMIT_FILE_SIZE" || err.code === "FST_ERR_MULTIPART_FILE_TOO_LARGE") {
      statusCode = 413;
      message = FILE_SIZE_EXCEEDS;
    } else if (err.code?.startsWith("FST_ERR_MULTIPART")) {
      statusCode = 400;
      message = INVALID_MULTIPART;
    } else if (statusCode === 413) {
      message = FILE_SIZE_EXCEEDS;
    }

    reply.code(statusCode);
    return makeError(message, details);
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
