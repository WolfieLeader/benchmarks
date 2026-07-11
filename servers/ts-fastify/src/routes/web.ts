import {
  env,
  INVALID_N,
  INVALID_TOKEN,
  makeError,
  parseComputeRounds,
  sha256Chain,
  signToken,
  VALIDATION_FAILED,
  validateWebPayload,
  verifyToken
} from "@bench/shared";
import type { FastifyInstance } from "fastify";

// The /html canon: greeting + fruit list + labeled total, rendered as a
// server-side template literal. The contract matches each value with
// htmlContains (whitespace-tolerant), not a byte-exact body.
const FRUITS = ["apple", "banana", "cherry"];

function renderHtml(): string {
  const items = FRUITS.map((fruit) => `    <li>${fruit}</li>`).join("\n");
  return `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>Benchmark</title></head>
<body>
  <h1>Hello, Alice</h1>
  <ul>
${items}
  </ul>
  <p>Total: 42</p>
</body>
</html>`;
}

export async function webRoutes(app: FastifyInstance) {
  app.get("/html", async (_request, reply) => {
    reply.type("text/html").send(renderHtml());
  });

  app.get("/jwt/sign", async () => {
    const token = await signToken(env.JWT_SECRET);
    return { token };
  });

  app.get("/jwt/verify", async (request, reply) => {
    const auth = request.headers.authorization ?? "";
    const token = auth.startsWith("Bearer ") ? auth.slice(7).trim() : "";
    if (!token) {
      reply.code(401);
      return makeError(INVALID_TOKEN, "missing bearer token");
    }
    try {
      return await verifyToken(env.JWT_SECRET, token);
    } catch (err) {
      reply.code(401);
      return makeError(INVALID_TOKEN, err);
    }
  });

  app.post("/validate", async (request, reply) => {
    const result = validateWebPayload(request.body);
    if (!result.ok) {
      reply.code(400);
      return makeError(VALIDATION_FAILED, result.details);
    }
    return { valid: true };
  });

  app.get<{ Querystring: { n?: string } }>("/compute", async (request, reply) => {
    const rounds = parseComputeRounds(request.query.n);
    if (rounds === null) {
      reply.code(400);
      return makeError(INVALID_N, "n must be an integer >= 1");
    }
    return { result: sha256Chain(rounds) };
  });
}
