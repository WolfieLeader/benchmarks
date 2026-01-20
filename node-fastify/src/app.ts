import fastify, { type FastifyInstance, type FastifyRequest } from "fastify";
import cookie from "@fastify/cookie";
import formbody from "@fastify/formbody";
import multipart, { type MultipartFile } from "@fastify/multipart";
import { paramsRoutes } from "./routes/params";
import { env } from "./env";

export const MAX_FILE_BYTES = 1 << 20; // 1MB
export const SNIFF_LEN = 512;
export const NULL_BYTE = 0x00;

export type FormFields = Record<string, string>;

export async function createApp(): Promise<FastifyInstance> {
  const app = fastify({ logger: env.ENV !== "prod" });

  await app.register(cookie);
  await app.register(formbody);
  await app.register(multipart, { limits: { fileSize: MAX_FILE_BYTES } });

  app.get("/", async (_request, reply) => {
    reply.send({ message: "Hello, World!" });
  });

  app.get("/health", async (_request, reply) => {
    reply.type("text/plain").send("OK");
  });

  await app.register(paramsRoutes, { prefix: "/params" });

  app.setNotFoundHandler(async (_req, reply) => {
    reply.code(404).send({ error: "not found" });
  });

  app.setErrorHandler(async (error, _request, reply) => {
    reply.code(500).send({ error: error.message || "internal error" });
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
