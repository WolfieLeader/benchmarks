// golangci full-copy drift gate: `node scripts/golangci-sync-check.mts`.
//
// The repo uses per-module FULL-COPY .golangci.json files (one complete
// standalone config per Go module — no shared base, no `extends`; each module
// is linted from its own directory by scripts/verify.mts). The upside is zero
// cross-module coupling; the cost is five copies that must not silently drift
// apart. This script is that guard: it structurally compares every copy against
// a reference and fails on any difference that is NOT a known, allowlisted
// per-module deviation.
//
// Wired into the `root` verify target (scripts/verify.mts) so `just verify` /
// `just verify root` fail on drift. npm-dependency-free (node: builtins only),
// matching scripts/biome-sync-check.mts. The configs are plain JSON (not JSONC),
// so a stray comment or trailing comma surfaces as a loud JSON.parse error.
//
// Deviation model differs from biome-sync-check: golangci copies carry keys that
// are LEGITIMATELY per-module (gofumpt module-path is different in every copy;
// the forbidigo boundary-rule scoping in linters.exclusions exists only in the
// two modules that need an exemption). So deviations are keyed by file → expected
// value (`byFile`): a file listed in byFile must carry exactly that value at the
// path; a file NOT listed must not carry the path at all. The path is stripped
// from every copy before the structural compare, so the rest must match exactly.

import { existsSync, readFileSync, readdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..");

// The exact set of Go modules that must carry the identical ladder. Adding a Go
// module means adding its .golangci.json here. The discovery sweep below asserts
// this list matches the .golangci.json files that actually exist, so a new copy
// can't silently skip the drift gate.
const CONFIGS = [
  "servers/go-stdlib/.golangci.json",
  "servers/go-chi/.golangci.json",
  "servers/go-gin/.golangci.json",
  "servers/go-fiber/.golangci.json",
  "servers/go-echo/.golangci.json",
  "benchmark/.golangci.json",
  "shared/go/.golangci.json"
] as const;

// The reference copy every other is compared against. Any drift surfaces as a
// diff between a copy and this one (if the reference itself drifts, every other
// copy reports against it — still a non-zero exit that points at the odd file).
const REFERENCE = "servers/go-chi/.golangci.json";

// Known, deliberate per-module deviations. At `path`: every file in `byFile`
// must carry exactly its listed value; every file NOT in `byFile` must NOT carry
// the path. The path is stripped from all copies before the structural compare.
type Deviation = { path: string[]; byFile: Record<string, unknown>; reason: string };
const DEVIATIONS: Deviation[] = [
  {
    path: ["formatters", "settings", "gofumpt", "module-path"],
    byFile: {
      "servers/go-stdlib/.golangci.json": "stdlib-server",
      "servers/go-chi/.golangci.json": "chi-server",
      "servers/go-gin/.golangci.json": "gin-server",
      "servers/go-fiber/.golangci.json": "fiber-server",
      "servers/go-echo/.golangci.json": "echo-server",
      "benchmark/.golangci.json": "benchmark-client",
      "shared/go/.golangci.json": "shared"
    },
    reason: "gofumpt module-path is the module's own import path — different in every module by definition"
  },
  {
    path: ["linters", "exclusions"],
    byFile: {
      // benchmark's CLI + summary packages ARE the sanctioned stdout surface, so
      // the forbidigo fmt.Print* ban is lifted there (and only there).
      "benchmark/.golangci.json": { rules: [{ path: "internal/cli/", linters: ["forbidigo"] }, { path: "internal/summary/", linters: ["forbidigo"] }] },
      // shared/go/config is the canonical env boundary, so the forbidigo os.Getenv
      // ban is lifted in that one file (and only there).
      "shared/go/.golangci.json": { rules: [{ path: "config/config.go", linters: ["forbidigo"] }] }
    },
    reason: "forbidigo boundary-rule scoping: only benchmark CLI/summary may fmt.Print*, only shared/go/config may read os.Getenv"
  }
];

const problems: string[] = [];
const add = (msg: string): void => void problems.push(msg);

// Discovery sweep: every .golangci.json under servers/*, shared/*, and the
// benchmark root must be registered in CONFIGS — an unregistered copy would lint
// by its own rules while this gate stays green, which is exactly the drift it
// exists to stop.
function discoverConfigs(): string[] {
  const found: string[] = [];
  for (const parent of ["servers", "shared"]) {
    for (const entry of readdirSync(join(repoRoot, parent), { withFileTypes: true })) {
      if (!entry.isDirectory()) continue;
      const rel = `${parent}/${entry.name}/.golangci.json`;
      if (existsSync(join(repoRoot, rel))) found.push(rel);
    }
  }
  if (existsSync(join(repoRoot, "benchmark/.golangci.json"))) found.push("benchmark/.golangci.json");
  return found;
}
const registered = new Set<string>(CONFIGS);
for (const rel of discoverConfigs()) {
  if (!registered.has(rel)) {
    add(`${rel}: .golangci.json exists but is not registered in CONFIGS — add it so the drift gate covers it`);
  }
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
    return JSON.parse(raw) as Record<string, unknown>;
  } catch (err) {
    add(`${rel}: invalid JSON — ${err instanceof Error ? err.message : String(err)}`);
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

// ── validate + strip the allowlisted per-file deviations ──────────────────────
for (const dev of DEVIATIONS) {
  const label = dev.path.join(".");
  for (const rel of CONFIGS) {
    const cfg = parsed.get(rel);
    if (!cfg) continue;
    const { found, value } = getPath(cfg, dev.path);
    const expectedForFile = rel in dev.byFile;
    if (expectedForFile) {
      const expected = dev.byFile[rel];
      if (!found) {
        add(`${rel}: expected allowlisted deviation "${label}" is missing (${dev.reason})`);
      } else if (JSON.stringify(value) !== JSON.stringify(expected)) {
        add(`${rel}: deviation "${label}" = ${JSON.stringify(value)}, expected ${JSON.stringify(expected)} (${dev.reason})`);
      }
    } else if (found) {
      add(`${rel}: unexpected deviation "${label}" — only allowed in ${Object.keys(dev.byFile).join(", ")} (${dev.reason})`);
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
    `\x1b[32m✓\x1b[0m golangci-sync-check: ${CONFIGS.length} full-copy .golangci.json configs in sync (allowlisted deviations only)`
  );
  process.exit(0);
}
console.error(`\x1b[1m\x1b[31m━━ golangci-sync-check: ${problems.length} drift problem(s) ━━\x1b[0m`);
for (const p of problems) console.error(`  \x1b[31m✗\x1b[0m ${p}`);
process.exit(1);
