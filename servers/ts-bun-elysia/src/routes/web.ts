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
import { Elysia } from "elysia";

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

export const webRouter = new Elysia();

webRouter.get("/html", ({ set }) => {
  set.headers["content-type"] = "text/html";
  return renderHtml();
});

webRouter.get("/jwt/sign", async () => {
  const token = await signToken(env.JWT_SECRET);
  return { token };
});

webRouter.get("/jwt/verify", async ({ headers, set }) => {
  const auth = headers.authorization ?? "";
  const token = auth.startsWith("Bearer ") ? auth.slice(7).trim() : "";
  if (!token) {
    set.status = 401;
    return makeError(INVALID_TOKEN, "missing bearer token");
  }
  try {
    return await verifyToken(env.JWT_SECRET, token);
  } catch (err) {
    set.status = 401;
    return makeError(INVALID_TOKEN, err);
  }
});

// /validate runs the SHARED zod rules (validateWebPayload), NOT Elysia's own
// `t.Object` schema. The validation rules are shared canon (PLAN §3 — one source
// of truth across every server); an Elysia `t` schema would duplicate them (drift
// risk) and its failure surfaces as Elysia's VALIDATION code, which app.ts maps
// to "invalid JSON body", not the required "validation failed". Handler-level
// validation is also this server's existing idiom (see routes/params.ts).
webRouter.post("/validate", ({ body, set }) => {
  const result = validateWebPayload(body);
  if (!result.ok) {
    set.status = 400;
    return makeError(VALIDATION_FAILED, result.details);
  }
  return { valid: true };
});

webRouter.get("/compute", ({ query, set }) => {
  const rounds = parseComputeRounds(query.n);
  if (rounds === null) {
    set.status = 400;
    return makeError(INVALID_N, "n must be an integer >= 1");
  }
  return { result: sha256Chain(rounds) };
});
