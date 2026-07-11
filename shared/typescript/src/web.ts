// web: the web-suite infrastructure shared across every TypeScript server that
// implements it (PLAN §3/§5): the canon constants for /compute and /jwt/sign,
// the shared /validate rules (one zod schema, the single source of truth), the
// SHA-256 compute chain, and the HS256 JWT sign/verify helpers (jose — the house
// pick). Only framework-independent contract values and security policy live
// here; the handlers (header extraction, response shaping, HTML rendering) stay
// per-framework and idiomatic (the §3 idiom boundary).

import { createHash } from "node:crypto";
import { jwtVerify, SignJWT } from "jose";
import { z } from "zod";

// ── /compute canon ──────────────────────────────────────────────────────────
// GET /compute applies SHA-256 to COMPUTE_SEED n times and returns the
// lowercase-hex digest. n must be an integer in [1, COMPUTE_MAX_ROUNDS]; above
// the cap it is clamped (bounds per-request CPU work). The seed must equal the
// conformance runner's $sha256chain seed (benchmark/internal/conformance).
export const COMPUTE_SEED = "benchmark";
export const COMPUTE_MAX_ROUNDS = 1_000_000;

// ── /jwt/sign canon ─────────────────────────────────────────────────────────
// GET /jwt/sign issues an HS256 token with these fixed claims plus a dynamic iat
// and exp (= iat + JWT_TTL_SECONDS), signed with the shared JWT_SECRET.
export const JWT_SUBJECT = "1234567890";
export const JWT_NAME = "John Doe";
export const JWT_ADMIN = true;
export const JWT_TTL_SECONDS = 3600; // canon TTL: 1 hour

// The five canon claims echoed by /jwt/verify, in the exact shape the strict
// contract body assertion expects (no extra keys).
export type WebTokenClaims = {
  sub: string;
  name: string;
  admin: boolean;
  iat: number;
  exp: number;
};

const secretKey = (secret: string): Uint8Array => new TextEncoder().encode(secret);

// Sign the canon token. exp is pinned to iat + JWT_TTL_SECONDS so the pair is
// exact (matches the other languages' 1-hour TTL), not merely "now-ish".
export async function signToken(secret: string): Promise<string> {
  const iat = Math.floor(Date.now() / 1000);
  return new SignJWT({ name: JWT_NAME, admin: JWT_ADMIN })
    .setProtectedHeader({ alg: "HS256", typ: "JWT" })
    .setSubject(JWT_SUBJECT)
    .setIssuedAt(iat)
    .setExpirationTime(iat + JWT_TTL_SECONDS)
    .sign(secretKey(secret));
}

// Verify a token cryptographically and echo the canon claims. The algorithm is
// pinned to HS256 (rejects alg:none / algorithm-confusion) and exp is required
// and validated (jose throws JWTExpired on an expired token) — mirrors Go's
// jwt.WithValidMethods(["HS256"]) + jwt.WithExpirationRequired(). Throws on any
// invalid token (bad signature, malformed, expired, missing exp); the caller
// turns that into a 401.
export async function verifyToken(secret: string, token: string): Promise<WebTokenClaims> {
  const { payload } = await jwtVerify(token, secretKey(secret), {
    algorithms: ["HS256"],
    requiredClaims: ["exp"]
  });
  // Reconstruct the exact five-claim shape from the verified payload so the echo
  // never carries an unexpected key (strict body match) and is fully typed.
  return {
    sub: String(payload.sub),
    name: String(payload.name),
    admin: Boolean(payload.admin),
    iat: Number(payload.iat),
    exp: Number(payload.exp)
  };
}

// ── /compute ────────────────────────────────────────────────────────────────
// Parse the raw `n` query value into a clamped round count, or null when it is
// missing / non-integer / < 1 / out of range (validate-at-boundary, house
// "invalid n" 400). Semantics mirror Go's strconv.Atoi exactly: ASCII digits
// with an optional leading sign only (the `\d` class is ASCII, so underscores
// and Unicode digits are rejected and no trimming happens), parsed as a signed
// 64-bit integer. BigInt does the range check because Number loses precision
// past 2^53 — e.g. 9300000000000000000 (> i64::MAX, < u64::MAX) must be rejected,
// which a Number-based check would wrongly clamp and accept.
const COMPUTE_I64_MAX = 9223372036854775807n;
export function parseComputeRounds(raw: string | null | undefined): number | null {
  if (raw == null || !/^[+-]?\d+$/.test(raw)) return null;
  const n = BigInt(raw);
  if (n < 1n || n > COMPUTE_I64_MAX) return null;
  return n > BigInt(COMPUTE_MAX_ROUNDS) ? COMPUTE_MAX_ROUNDS : Number(n);
}

// Apply SHA-256 to the seed bytes `rounds` times and return the lowercase-hex
// digest. Web Crypto's subtle.digest is async, useless for a tight million-round
// chain; node:crypto's synchronous createHash is the right tool and is supported
// identically on Node, Bun, and Deno. Run inline on the request path (no worker
// offload) so the single-process shape matches every other language's /compute
// (fairness — typescript.md rule 21).
export function sha256Chain(rounds: number): string {
  let digest = createHash("sha256").update(COMPUTE_SEED).digest();
  for (let i = 1; i < rounds; i++) {
    digest = createHash("sha256").update(digest).digest();
  }
  return digest.toString("hex");
}

// ── /validate ───────────────────────────────────────────────────────────────
// The ~4-level POST /validate schema (contract/web.json canon). Kept module-local
// (not exported) so oxc isolated-declarations needs no annotation on it; the
// exported surface is the validate() helper, which is the shared rule set every
// server runs — the single source of truth (PLAN §3: validation rules are shared
// infrastructure, so no framework re-declares them in its own validator).
//
// Optionality mirrors the Go reference (shared/go/web ValidatePayload) exactly:
// fields Go only range-checks (age gte=0,lte=120 / total gte=0 / tags untagged)
// are zero-valued by encoding/json when omitted and still validate — zod
// `.default()` reproduces that. Fields Go marks `required` (user, id, email,
// profile, role, preferences, notifications, theme, items min=1, sku — and
// quantity, whose gte=1 rejects the zero value anyway) stay required here too.
const zValidatePayload = z.object({
  user: z.object({
    id: z.uuid(),
    email: z.email(),
    profile: z.object({
      age: z.number().int().min(0).max(120).default(0),
      role: z.enum(["admin", "user", "guest"]),
      preferences: z.object({
        theme: z.enum(["light", "dark"]),
        notifications: z.boolean()
      })
    })
  }),
  items: z
    .array(
      z.object({
        sku: z.string().min(1),
        quantity: z.number().int().min(1).max(100),
        tags: z.array(z.string()).default([])
      })
    )
    .min(1),
  total: z.number().min(0).default(0)
});

// The result of running the shared /validate rules: ok, or a per-framework
// details summary (the contract asserts $present, not an exact error count —
// zod/pydantic/validator/serde count failures differently).
export type ValidateResult = { ok: true } | { ok: false; details: string };

export function validateWebPayload(input: unknown): ValidateResult {
  const parsed = zValidatePayload.safeParse(input);
  return parsed.success ? { ok: true } : { ok: false, details: parsed.error.message };
}
