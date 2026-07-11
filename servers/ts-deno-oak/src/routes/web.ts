import {
  env,
  INVALID_JSON_BODY,
  INVALID_N,
  INVALID_TOKEN,
  makeError,
  parseComputeRounds,
  sha256Chain,
  signToken,
  validateWebPayload,
  VALIDATION_FAILED,
  verifyToken
} from "@bench/shared";
import { Router } from "@oak/oak";

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

export const webRoutes = new Router();

webRoutes.get("/html", (ctx) => {
  ctx.response.type = "text/html";
  ctx.response.body = renderHtml();
});

webRoutes.get("/jwt/sign", async (ctx) => {
  const token = await signToken(env.JWT_SECRET);
  ctx.response.body = { token };
});

webRoutes.get("/jwt/verify", async (ctx) => {
  const auth = ctx.request.headers.get("Authorization") ?? "";
  const token = auth.startsWith("Bearer ") ? auth.slice(7).trim() : "";
  if (!token) {
    ctx.response.status = 401;
    ctx.response.body = makeError(INVALID_TOKEN, "missing bearer token");
    return;
  }
  try {
    ctx.response.body = await verifyToken(env.JWT_SECRET, token);
  } catch (err) {
    ctx.response.status = 401;
    ctx.response.body = makeError(INVALID_TOKEN, err);
  }
});

webRoutes.post("/validate", async (ctx) => {
  let body: unknown;
  try {
    body = await ctx.request.body.json();
  } catch (err) {
    ctx.response.status = 400;
    ctx.response.body = makeError(INVALID_JSON_BODY, err);
    return;
  }

  const result = validateWebPayload(body);
  if (!result.ok) {
    ctx.response.status = 400;
    ctx.response.body = makeError(VALIDATION_FAILED, result.details);
    return;
  }
  ctx.response.body = { valid: true };
});

webRoutes.get("/compute", (ctx) => {
  const rounds = parseComputeRounds(ctx.request.url.searchParams.get("n"));
  if (rounds === null) {
    ctx.response.status = 400;
    ctx.response.body = makeError(INVALID_N, "n must be an integer >= 1");
    return;
  }
  ctx.response.body = { result: sha256Chain(rounds) };
});
