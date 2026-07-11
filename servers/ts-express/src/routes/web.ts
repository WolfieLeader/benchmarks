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
import express, { type Request, type Response, type Router } from "express";

// The /html canon: a greeting, a fruit list, and a labeled total, rendered as a
// server-side template (a literal interpolating the values). The contract matches
// each value with htmlContains (whitespace-tolerant), not a byte-exact body.
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

// Strip the "Bearer " prefix off the Authorization header; returns the trimmed
// token or "" when absent/empty (-> 401 invalid token).
function bearerToken(req: Request): string {
  const auth = req.get("authorization") ?? "";
  return auth.startsWith("Bearer ") ? auth.slice(7).trim() : "";
}

export const webRouter: Router = express.Router();

webRouter.get("/html", (_req: Request, res: Response) => {
  res.type("text/html").send(renderHtml());
});

webRouter.get("/jwt/sign", async (_req: Request, res: Response) => {
  const token = await signToken(env.JWT_SECRET);
  res.json({ token });
});

webRouter.get("/jwt/verify", async (req: Request, res: Response) => {
  const token = bearerToken(req);
  if (!token) {
    res.status(401).json(makeError(INVALID_TOKEN, "missing bearer token"));
    return;
  }
  try {
    const claims = await verifyToken(env.JWT_SECRET, token);
    res.json(claims);
  } catch (err) {
    res.status(401).json(makeError(INVALID_TOKEN, err));
  }
});

webRouter.post("/validate", (req: Request, res: Response) => {
  const result = validateWebPayload(req.body);
  if (!result.ok) {
    res.status(400).json(makeError(VALIDATION_FAILED, result.details));
    return;
  }
  res.json({ valid: true });
});

webRouter.get("/compute", (req: Request, res: Response) => {
  const nValue = Array.isArray(req.query.n) ? req.query.n[0] : req.query.n;
  const rounds = parseComputeRounds(typeof nValue === "string" ? nValue : undefined);
  if (rounds === null) {
    res.status(400).json(makeError(INVALID_N, "n must be an integer >= 1"));
    return;
  }
  res.json({ result: sha256Chain(rounds) });
});
