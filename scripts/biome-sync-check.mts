// Biome full-copy drift gate (Phase 1): `node scripts/biome-sync-check.mts`.
//
// The repo uses per-project FULL-COPY biome.jsonc files (Go-style, one complete
// standalone config per Biome-linted TS project — no root config, no `extends`;
// see any biome.jsonc header and PLAN §11.1 law 2). The upside is zero root
// lane-contention; the cost is six copies that must not silently drift apart.
// This script is that guard: it structurally compares every copy against a
// reference and fails on any difference that is NOT a known, allowlisted
// per-project deviation.
//
// Wired into the `root` verify target (scripts/verify.mts) so `just verify` /
// `just verify root` fail on drift. npm-dependency-free (node: builtins only),
// matching scripts/contract.mts — it parses JSONC by stripping comments, no
// external parser.

import { existsSync, readFileSync, readdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..");

// The exact set of projects that must carry the identical ladder. Adding a
// Biome-linted TS project means adding its biome.jsonc here (ts-deno-oak is
// deliberately absent — it uses deno lint). The discovery sweep below asserts
// this list matches the biome.jsonc files that actually exist, so a new copy
// can't silently skip the drift gate.
const CONFIGS = [
  "servers/ts-express/biome.jsonc",
  "servers/ts-fastify/biome.jsonc",
  "servers/ts-nestjs/biome.jsonc",
  "servers/ts-bun-elysia/biome.jsonc",
  "servers/ts-honojs/biome.jsonc",
  "servers/ts-bun-honojs/biome.jsonc",
  "shared/typescript/biome.jsonc",
  "shared/typescript-hono/biome.jsonc"
] as const;

// The reference copy every other is compared against. Any drift surfaces as a
// diff between a copy and this one (if the reference itself drifts, every other
// copy reports against it — still a non-zero exit that points at the odd file).
const REFERENCE = "servers/ts-express/biome.jsonc";

// Known, deliberate per-project deviations. A key at `path` may appear ONLY in
// `allowedFiles`, and only with `expected` value; everywhere else it is drift.
// These paths are stripped before the structural compare so the rest must match
// byte-for-byte (after comment removal). NOTE: the two Bun servers carry NO
// Bun-specific Biome keys — their only Bun concession is the repo-wide
// noUnresolvedImports:off (bun-module named exports), which lives in every copy.
type Deviation = { path: string[]; allowedFiles: string[]; expected: unknown; reason: string };
const DEVIATIONS: Deviation[] = [
  {
    path: ["javascript", "parser"],
    allowedFiles: ["servers/ts-nestjs/biome.jsonc"],
    expected: { unsafeParameterDecoratorsEnabled: true },
    reason: "NestJS parameter decorators (@Param()/@Body()) require Biome to parse them"
  }
];

const problems: string[] = [];
const add = (msg: string): void => void problems.push(msg);

// Discovery sweep: every biome.jsonc under servers/* and shared/* must be
// registered in CONFIGS — an unregistered copy would lint by its own rules
// while this gate stays green, which is exactly the drift it exists to stop.
function discoverConfigs(): string[] {
  const found: string[] = [];
  for (const parent of ["servers", "shared"]) {
    for (const entry of readdirSync(join(repoRoot, parent), { withFileTypes: true })) {
      if (!entry.isDirectory()) continue;
      const rel = `${parent}/${entry.name}/biome.jsonc`;
      if (existsSync(join(repoRoot, rel))) found.push(rel);
    }
  }
  return found;
}
const registered = new Set<string>(CONFIGS);
for (const rel of discoverConfigs()) {
  if (!registered.has(rel)) {
    add(`${rel}: biome.jsonc exists but is not registered in CONFIGS — add it so the drift gate covers it`);
  }
}

// Strip `//` line and `/* */` block comments while respecting string context, so
// `"https://..."` and message strings survive intact. Trailing commas are not
// stripped — a stray one surfaces as a loud JSON.parse error (correct + rare,
// since these configs use trailingCommas:none).
function stripJsonc(src: string): string {
  let out = "";
  let inString = false;
  for (let i = 0; i < src.length; i++) {
    const c = src[i];
    if (inString) {
      out += c;
      if (c === "\\") {
        out += src[i + 1] ?? "";
        i++;
      } else if (c === '"') {
        inString = false;
      }
      continue;
    }
    if (c === '"') {
      inString = true;
      out += c;
      continue;
    }
    if (c === "/" && src[i + 1] === "/") {
      i += 2;
      while (i < src.length && src[i] !== "\n") i++;
      continue;
    }
    if (c === "/" && src[i + 1] === "*") {
      i += 2;
      while (i < src.length && !(src[i] === "*" && src[i + 1] === "/")) i++;
      i++;
      continue;
    }
    out += c;
  }
  return out;
}

function parseConfig(rel: string): Record<string, unknown> | null {
  let raw: string;
  try {
    raw = readFileSync(join(repoRoot, rel), "utf8");
  } catch (err) {
    add(`${rel}: cannot read file — ${err instanceof Error ? err.message : String(err)}`);
    return null;
  }
  try {
    return JSON.parse(stripJsonc(raw)) as Record<string, unknown>;
  } catch (err) {
    add(`${rel}: invalid JSONC — ${err instanceof Error ? err.message : String(err)}`);
    return null;
  }
}

// Deep structural diff — records every mismatch with a dotted/indexed path.
function diff(ref: unknown, other: unknown, path: string, out: string[]): void {
  if (ref === other) return;
  if (ref === null || other === null || typeof ref !== "object" || typeof other !== "object") {
    out.push(`${path || "(root)"}: reference=${JSON.stringify(ref)} vs this=${JSON.stringify(other)}`);
    return;
  }
  const refArr = Array.isArray(ref);
  const otherArr = Array.isArray(other);
  if (refArr !== otherArr) {
    out.push(`${path || "(root)"}: array-vs-object shape mismatch`);
    return;
  }
  const keys = new Set([...Object.keys(ref), ...Object.keys(other)]);
  for (const k of keys) {
    const p = refArr ? `${path}[${k}]` : path ? `${path}.${k}` : k;
    const inRef = k in (ref as Record<string, unknown>);
    const inOther = k in (other as Record<string, unknown>);
    if (!inRef) {
      out.push(`${p}: present in this copy but not in reference`);
      continue;
    }
    if (!inOther) {
      out.push(`${p}: present in reference but missing from this copy`);
      continue;
    }
    diff((ref as Record<string, unknown>)[k], (other as Record<string, unknown>)[k], p, out);
  }
}

function getPath(obj: Record<string, unknown>, path: string[]): { found: boolean; value: unknown } {
  let cur: unknown = obj;
  for (const key of path) {
    if (cur === null || typeof cur !== "object" || !(key in cur)) return { found: false, value: undefined };
    cur = (cur as Record<string, unknown>)[key];
  }
  return { found: true, value: cur };
}

function deletePath(obj: Record<string, unknown>, path: string[]): void {
  let cur: unknown = obj;
  for (let i = 0; i < path.length - 1; i++) {
    if (cur === null || typeof cur !== "object") return;
    cur = (cur as Record<string, unknown>)[path[i]];
  }
  if (cur && typeof cur === "object") delete (cur as Record<string, unknown>)[path[path.length - 1]];
}

// ── load every copy ──────────────────────────────────────────────────────────
const parsed = new Map<string, Record<string, unknown>>();
for (const rel of CONFIGS) {
  const cfg = parseConfig(rel);
  if (cfg !== null) parsed.set(rel, cfg);
}

// ── validate + strip the allowlisted deviations ──────────────────────────────
for (const dev of DEVIATIONS) {
  const label = dev.path.join(".");
  for (const rel of CONFIGS) {
    const cfg = parsed.get(rel);
    if (!cfg) continue;
    const { found, value } = getPath(cfg, dev.path);
    const allowed = dev.allowedFiles.includes(rel);
    if (found && !allowed) {
      add(`${rel}: unexpected deviation "${label}" — only allowed in ${dev.allowedFiles.join(", ")} (${dev.reason})`);
    } else if (found && JSON.stringify(value) !== JSON.stringify(dev.expected)) {
      add(`${rel}: deviation "${label}" = ${JSON.stringify(value)}, expected ${JSON.stringify(dev.expected)}`);
    } else if (!found && allowed) {
      add(`${rel}: expected allowlisted deviation "${label}" is missing (${dev.reason})`);
    }
    // Strip it from every copy so the structural compare ignores allowlisted keys.
    deletePath(cfg, dev.path);
  }
}

// ── structural compare against the reference ─────────────────────────────────
const ref = parsed.get(REFERENCE);
if (!ref) {
  add(`reference config ${REFERENCE} could not be loaded — cannot compare copies`);
} else {
  for (const rel of CONFIGS) {
    if (rel === REFERENCE) continue;
    const cfg = parsed.get(rel);
    if (!cfg) continue;
    const out: string[] = [];
    diff(ref, cfg, "", out);
    for (const d of out) add(`${rel} vs ${REFERENCE}: ${d}`);
  }
}

// ── report ───────────────────────────────────────────────────────────────────
if (problems.length === 0) {
  console.log(
    `\x1b[32m✓\x1b[0m biome-sync-check: ${CONFIGS.length} full-copy biome.jsonc configs in sync (allowlisted deviations only)`
  );
  process.exit(0);
}
console.error(`\x1b[1m\x1b[31m━━ biome-sync-check: ${problems.length} drift problem(s) ━━\x1b[0m`);
for (const p of problems) console.error(`  \x1b[31m✗\x1b[0m ${p}`);
process.exit(1);
