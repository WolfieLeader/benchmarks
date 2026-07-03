// Config + manifest consistency gate (Phase 0B): `just check-config`.
//
// Three checks, all reported together (never bail on the first error):
//   (a) config/config.json           validates against config/config.schema.json
//   (b) http-servers/**/bench.json    each validates against config/bench.schema.json
//   (c) cross-consistency            every config.json server has a matching manifest
//       and vice versa (name/image/port equality); every manifest database is one
//       of config.json's databases.
//
// This is a verify-time dev tool, so it MAY use ajv (a root devDependency) to
// validate against the real schema files — the single source of truth — rather
// than re-implementing draft-07 by hand. scripts/contract.mts stays dependency-free.
//
//   node scripts/check-config.mts

import { existsSync, readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { Ajv, type ErrorObject } from "ajv";
import addFormats from "ajv-formats";

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
const httpServersDir = join(repoRoot, "http-servers");
const rel = (p: string): string => relative(repoRoot, p);

const problems: string[] = [];
const add = (msg: string): void => void problems.push(msg);

// Read + JSON.parse a file, recording a precise problem (and returning null) on
// a missing file or a syntax error so the run can continue with what it has.
function readJson(path: string): unknown {
  let raw: string;
  try {
    raw = readFileSync(path, "utf8");
  } catch (err) {
    add(`${rel(path)}: cannot read file — ${err instanceof Error ? err.message : String(err)}`);
    return null;
  }
  try {
    return JSON.parse(raw);
  } catch (err) {
    add(`${rel(path)}: invalid JSON — ${err instanceof Error ? err.message : String(err)}`);
    return null;
  }
}

const ajv = new Ajv({ allErrors: true, allowUnionTypes: true });
addFormats(ajv);

function formatAjvError(file: string, e: ErrorObject): string {
  const loc = e.instancePath || "(root)";
  let extra = "";
  if (e.keyword === "additionalProperties") extra = ` — unexpected property "${e.params.additionalProperty}"`;
  else if (e.keyword === "enum" && Array.isArray(e.params.allowedValues)) {
    extra = ` (allowed: ${e.params.allowedValues.join(", ")})`;
  }
  return `${file} ${loc}: ${e.message}${extra}`;
}

// Validate `data` against the schema at `schemaPath`; record every violation.
function validate(schemaPath: string, dataPath: string, data: unknown): boolean {
  const schema = readJson(schemaPath);
  if (schema === null) return false;
  const validateFn = ajv.compile(schema as object);
  if (validateFn(data)) return true;
  for (const e of validateFn.errors ?? []) add(formatAjvError(rel(dataPath), e));
  return false;
}

// ── (a) config/config.json ────────────────────────────────────────────────
const configPath = join(repoRoot, "config", "config.json");
const configSchemaPath = join(repoRoot, "config", "config.schema.json");
const config = readJson(configPath);
if (config !== null) validate(configSchemaPath, configPath, config);

type ConfigServer = { name: string; image: string; port: number };
const configServers = (config as { servers?: ConfigServer[] } | null)?.servers ?? [];
const configDatabases = (config as { databases?: string[] } | null)?.databases ?? [];

// ── (b) http-servers/**/bench.json ──────────────────────────────────────────
const benchSchemaPath = join(repoRoot, "config", "bench.schema.json");
// Fixed two-level walk (http-servers/<lang>/<entry>/bench.json) — never a
// recursive scan, so installed dependency trees (node_modules/.venv/dist)
// can't inject stray bench.json files. Deliberately NOT shared with lib.mts:
// importing lib would run its fail-fast discovery at module load, killing this
// script before it can report a malformed manifest gracefully.
const manifestFiles: string[] = [];
for (const lang of readdirSync(httpServersDir, { withFileTypes: true })) {
  if (!lang.isDirectory()) continue;
  for (const entry of readdirSync(join(httpServersDir, lang.name), { withFileTypes: true })) {
    if (!entry.isDirectory()) continue;
    const manifest = join(httpServersDir, lang.name, entry.name, "bench.json");
    if (existsSync(manifest)) manifestFiles.push(manifest);
  }
}
manifestFiles.sort();

if (manifestFiles.length === 0) add(`no bench.json manifests found under ${rel(httpServersDir)}/`);

type Manifest = { name: string; image: string; port: number; databases: string[] };
const manifests: { file: string; data: Manifest }[] = [];
const nameToFile = new Map<string, string>();

for (const file of manifestFiles) {
  const data = readJson(file);
  if (data === null) continue;
  const valid = validate(benchSchemaPath, file, data);
  const m = data as Manifest;
  // A duplicate name breaks discovery's single-source-of-truth guarantee.
  if (typeof m.name === "string") {
    const prior = nameToFile.get(m.name);
    if (prior) add(`duplicate server name "${m.name}" in ${rel(file)} and ${prior}`);
    else nameToFile.set(m.name, rel(file));
  }
  if (valid) manifests.push({ file, data: m });
}

// ── (c) cross-consistency: config.json servers[] <-> bench.json manifests ────
const manifestByName = new Map(manifests.map((m) => [m.data.name, m]));
const configByName = new Map(configServers.map((s) => [s.name, s]));

for (const s of configServers) {
  const m = manifestByName.get(s.name);
  if (!m) {
    add(`${rel(configPath)} servers[]: "${s.name}" has no matching bench.json manifest`);
    continue;
  }
  if (m.data.image !== s.image) {
    add(`image mismatch for "${s.name}": ${rel(configPath)}=${s.image} vs ${rel(m.file)}=${m.data.image}`);
  }
  if (m.data.port !== s.port) {
    add(`port mismatch for "${s.name}": ${rel(configPath)}=${s.port} vs ${rel(m.file)}=${m.data.port}`);
  }
}

for (const m of manifests) {
  if (!configByName.has(m.data.name)) {
    add(`${rel(m.file)}: server "${m.data.name}" has no matching entry in ${rel(configPath)} servers[]`);
  }
  for (const db of m.data.databases ?? []) {
    if (!configDatabases.includes(db)) {
      add(
        `${rel(m.file)} databases[]: "${db}" is not one of ${rel(configPath)} databases [${configDatabases.join(", ")}]`
      );
    }
  }
}

// ── report ────────────────────────────────────────────────────────────────
if (problems.length === 0) {
  const n = manifests.length;
  console.log(`\x1b[32m✓\x1b[0m check-config: config.json + ${n} manifest${n === 1 ? "" : "s"} valid and consistent`);
  process.exit(0);
}
console.error(`\x1b[1m\x1b[31m━━ check-config: ${problems.length} problem(s) ━━\x1b[0m`);
for (const p of problems) console.error(`  \x1b[31m✗\x1b[0m ${p}`);
process.exit(1);
