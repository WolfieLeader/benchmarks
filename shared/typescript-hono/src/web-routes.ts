// /web routes: the five web-suite endpoints (/html, /jwt/sign, /jwt/verify,
// /validate, /compute), shared by the three Hono runtime entries (ts-honojs on
// Node, ts-bun-honojs on Bun, ts-deno-honojs on Deno). All the logic — the
// validate rules, the SHA-256 chain, the jose JWT helpers — lives in @bench/shared;
// these handlers are the Hono-idiomatic glue only (web-standard Request/Response,
// so identical across runtimes).

import {
  env,
  INVALID_JSON_BODY,
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
import { Hono } from "hono";

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

export function createWebRoutes(): Hono {
  const webRoutes = new Hono();

  webRoutes.get("/html", (c) => c.html(renderHtml()));

  webRoutes.get("/jwt/sign", async (c) => {
    const token = await signToken(env.JWT_SECRET);
    return c.json({ token });
  });

  webRoutes.get("/jwt/verify", async (c) => {
    const auth = c.req.header("Authorization") ?? "";
    const token = auth.startsWith("Bearer ") ? auth.slice(7).trim() : "";
    if (!token) return c.json(makeError(INVALID_TOKEN, "missing bearer token"), 401);
    try {
      const claims = await verifyToken(env.JWT_SECRET, token);
      return c.json(claims);
    } catch (err) {
      return c.json(makeError(INVALID_TOKEN, err), 401);
    }
  });

  webRoutes.post("/validate", async (c) => {
    let body: unknown;
    try {
      body = await c.req.json();
    } catch (err) {
      return c.json(makeError(INVALID_JSON_BODY, err), 400);
    }

    const result = validateWebPayload(body);
    if (!result.ok) return c.json(makeError(VALIDATION_FAILED, result.details), 400);
    return c.json({ valid: true });
  });

  webRoutes.get("/compute", (c) => {
    const rounds = parseComputeRounds(c.req.query("n"));
    if (rounds === null) return c.json(makeError(INVALID_N, "n must be an integer >= 1"), 400);
    return c.json({ result: sha256Chain(rounds) });
  });

  return webRoutes;
}
