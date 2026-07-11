import cookie from "@fastify/cookie";
import formbody from "@fastify/formbody";
import multipart from "@fastify/multipart";
import fastify, { type FastifyError, type FastifyInstance } from "fastify";
import {
  env,
  FILE_SIZE_EXCEEDS,
  INTERNAL_ERROR,
  INVALID_JSON_BODY,
  INVALID_MULTIPART,
  makeError,
  MAX_FILE_BYTES,
  MAX_REQUEST_BYTES,
  NOT_FOUND
} from "@bench/shared";
import { dbRoutes } from "./routes/db.js";
import { paramsRoutes } from "./routes/params.js";
import { webRoutes } from "./routes/web.js";

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

  // Error/not-found handlers must be registered BEFORE the route plugins:
  // handlers only propagate into encapsulated child contexts that are created
  // *after* they are set, and Fastify's body/content-type parser errors are
  // raised inside each route plugin's context. Registering these last would
  // leave those errors falling through to Fastify's default serializer
  // (e.g. malformed JSON -> "Bad Request" instead of "invalid JSON body").
  app.setNotFoundHandler(async (_req, reply) => {
    reply.code(404);
    return { error: NOT_FOUND };
  });

  app.setErrorHandler<FastifyError>(async (error, _req, reply) => {
    let statusCode = error.statusCode ?? 500;
    let message = INTERNAL_ERROR;

    if (error.code === "FST_ERR_CTP_INVALID_JSON_BODY" || error.code === "FST_ERR_CTP_EMPTY_JSON_BODY") {
      statusCode = 400;
      message = INVALID_JSON_BODY;
    } else if (
      error.code === "FST_ERR_CTP_BODY_TOO_LARGE" ||
      error.code === "FST_REQ_FILE_TOO_LARGE" ||
      error.code === "FST_PARTS_LIMIT" ||
      error.code === "FST_FILES_LIMIT" ||
      error.code === "FST_FIELDS_LIMIT"
    ) {
      statusCode = 413;
      message = FILE_SIZE_EXCEEDS;
    } else if (error.code?.startsWith("FST_INVALID_MULTIPART") || error.code === "FST_NO_FORM_DATA") {
      statusCode = 400;
      message = INVALID_MULTIPART;
    } else if (statusCode === 413) {
      message = FILE_SIZE_EXCEEDS;
    }

    reply.code(statusCode);
    return makeError(message, error.message || undefined);
  });

  app.get("/", async () => {
    return { hello: "world" };
  });
  app.get("/health", async (_req, reply) => {
    reply.type("text/plain").send("OK");
  });

  await app.register(paramsRoutes, { prefix: "/params" });
  await app.register(dbRoutes, { prefix: "/db" });
  await app.register(webRoutes);

  return app;
}
