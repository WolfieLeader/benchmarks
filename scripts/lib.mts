// Shared helpers for the scripts/ orchestrators (Phase 0B).
//
// One roster table (SERVERS) is the single source of truth for the target list;
// each script maps its rows to concrete shell commands. A bounded-concurrency
// runner executes per-target jobs in parallel and a grouped report prints at the
// end instead of bailing on the first failure (`a && b && c` hides later errors).
//
// Run with Node 26 native type-stripping — erasable-syntax TS only, no build step.

import { spawn, spawnSync } from "node:child_process";
import { existsSync, readdirSync, readFileSync } from "node:fs";
import { homedir } from "node:os";
import { dirname, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

export const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
const serversDir = join(repoRoot, "servers");

// NOTE: the two process.env mutations below run at module load — they are an
// IMPORT SIDE EFFECT, not a function. Every runtime importer of lib.mts gets
// them for free, but a future *type-only* import (`import type { … }`) is erased
// by Node's type-stripping and would skip them entirely; a Go command spawned
// off such a path would miss the toolchain pin. Keep any consumer that spawns
// `go`/`golangci-lint` on a value import of this module.

// Go toolchain pin (PLAN §0.1, §10). Every Go command the scripts spawn — build,
// go run, go get/tidy, the conformance binary in contract.mts — must resolve the
// Go 1.27rc1 toolchain (each go.mod's `go 1.27rc1` directive already forces this
// under the default GOTOOLCHAIN=auto; the explicit pin makes it deterministic).
// json/v2 is in the RC's default baseline — no GOEXPERIMENT needed. This var
// only affects `go`; other ecosystems (pnpm/bun/deno/uv) ignore it.
process.env.GOTOOLCHAIN ??= "go1.27rc1";
// golangci-lint refuses to load any module targeting a Go version newer than
// the Go it was built with, so the brew bottle (built with go1.26.x) cannot
// lint the go-1.27rc1 modules. A rebuild lives in ~/go/bin (PLAN §0.1:
// `GOTOOLCHAIN=go1.27rc1 go install .../golangci-lint/v2/cmd/golangci-lint@<pinned ver>`);
// prepend it so it outranks the brew one for every command these scripts spawn.
// Blast radius: this prepend is process-wide, so it fronts ~/go/bin for EVERY
// spawned command (pnpm/bun/deno/uv/docker included), not just Go — deliberate
// but not narrowed, since runStep spawns bare shell strings with no per-command
// env and threading a Go-only PATH through would ripple across all callers. The
// only ~/go/bin binaries in play are Go tools, so the wider scope is inert here.
process.env.PATH = `${join(homedir(), "go", "bin")}:${process.env.PATH ?? ""}`;
// Rust toolchain pin (PLAN §0.1): the Homebrew `rustup` formula is keg-only and
// there is NO `~/.cargo/bin`, so the `cargo`/`rustc`/`rustfmt`/`cargo-clippy`
// proxies live only in /opt/homebrew/opt/rustup/bin. Prepend it (when present)
// so every cargo command these scripts spawn resolves without relying on the
// dev's shell profile. Inert on Linux/CI where the dir does not exist.
const rustupBin = "/opt/homebrew/opt/rustup/bin";
if (existsSync(rustupBin)) process.env.PATH = `${rustupBin}:${process.env.PATH ?? ""}`;

export type Eco = "pnpm" | "bun" | "deno" | "uv" | "go" | "zig" | "cargo" | "root";

export type Server = {
  name: string; // CLI target key (e.g. "ts-express", "go-chi")
  dir: string; // working directory for its commands
  eco: Eco; // toolchain family — commands derive from this per script
  image?: string; // docker image tag; present => included in `images`
  dev?: string; // dev command; present => included in `dev`
  goBin?: string; // eco "go": build output name (default "server")
  lib?: boolean; // eco "go": library module (no ./cmd/main.go) — build ./... instead
  port?: number; // host/container port (from the manifest; servers only)
};

// Manifest runtime -> toolchain family, and family -> dev command. These are the
// only manifest-derived mappings the scripts need; everything else on a discovered
// row (name/dir/image/port) comes straight from bench.json.
const RUNTIME_ECO: Record<string, Eco> = {
  node: "pnpm",
  bun: "bun",
  deno: "deno",
  go: "go",
  python: "uv",
  zig: "zig",
  rust: "cargo"
};
const ECO_DEV: Partial<Record<Eco, string>> = {
  pnpm: "pnpm run dev",
  bun: "bun run dev",
  deno: "deno task dev",
  go: "air",
  uv: "uv run python -m src.main",
  zig: "zig build run",
  cargo: "cargo run"
};

type Manifest = {
  name: string;
  language: string;
  runtime: string;
  image: string;
  port: number;
  databases: string[];
  experimental: boolean;
  dev_port: number;
};

function fatal(msg: string): never {
  console.error(`\x1b[31m✗\x1b[0m ${msg}`);
  process.exit(1);
}

// Manifests live exactly at servers/<entry>/bench.json (flat layout, PLAN §2.1).
// A fixed one-level walk (never a recursive scan) cannot descend into installed
// dependency trees (node_modules/.venv/dist), where a stray file named
// bench.json would otherwise kill every script repo-wide.
function manifestPaths(): string[] {
  const found: string[] = [];
  for (const entry of readdirSync(serversDir, { withFileTypes: true })) {
    if (!entry.isDirectory()) continue;
    const manifest = join(serversDir, entry.name, "bench.json");
    if (existsSync(manifest)) found.push(manifest);
  }
  return found.sort();
}

// Discover the server roster by scanning servers/*/bench.json — adding a
// server = adding a folder with a manifest, zero central edits (PLAN §7.4). This
// is the single source of truth for the roster; there is NO static fallback list.
// Structural guardrails here are minimal (JSON parses, required fields present,
// known runtime, unique names) so any script fails loud on a broken manifest;
// full schema + config cross-checks live in scripts/check-config.mts.
function discoverServers(): Server[] {
  const found = manifestPaths();
  if (found.length === 0) fatal(`no bench.json manifests found under ${relative(repoRoot, serversDir)}/`);

  const servers: Server[] = [];
  const seen = new Map<string, string>(); // name -> first manifest that declared it
  for (const file of found) {
    const rel = relative(repoRoot, file);
    let m: Manifest;
    try {
      m = JSON.parse(readFileSync(file, "utf8")) as Manifest;
    } catch (err) {
      fatal(`malformed manifest ${rel}: ${err instanceof Error ? err.message : String(err)}`);
    }
    for (const key of ["name", "runtime", "image", "port"] as const) {
      if (m[key] === undefined || m[key] === null) fatal(`manifest ${rel}: missing required field "${key}"`);
    }
    const eco = RUNTIME_ECO[m.runtime];
    if (!eco) {
      fatal(`manifest ${rel}: unknown runtime "${m.runtime}" (expected ${Object.keys(RUNTIME_ECO).join(", ")})`);
    }
    // Keep this acceptance predicate in sync with benchmark/internal/roster —
    // the two discoverers must agree on what a valid manifest is.
    if (m.name.trim() === "") fatal(`manifest ${rel}: "name" must be non-empty`);
    if (!Number.isInteger(m.port) || m.port < 1 || m.port > 65535) {
      fatal(`manifest ${rel}: port must be between 1 and 65535, got ${m.port}`);
    }
    const prior = seen.get(m.name);
    if (prior) fatal(`duplicate server name "${m.name}" in ${rel} and ${prior}`);
    seen.set(m.name, rel);
    const priorImage = seen.get(`image:${m.image}`);
    if (priorImage) fatal(`duplicate image "${m.image}" in ${rel} and ${priorImage}`);
    seen.set(`image:${m.image}`, rel);
    servers.push({ name: m.name, dir: dirname(file), eco, image: m.image, port: m.port, dev: ECO_DEV[eco] });
  }
  return servers;
}

// Non-server targets the scripts also drive: the Go load generator and the repo
// root (prettier/config checks). These carry no bench.json manifest, so they are
// static rows appended after discovery — not a roster fallback.
const EXTRA_TARGETS: Server[] = [
  { name: "benchmark", dir: join(repoRoot, "benchmark"), eco: "go", goBin: "benchmark" },
  // Shared Go module consumed by the go-* servers via a path `replace` (PLAN §3/§2.2).
  // It is a library (no ./cmd/main.go), so verify builds `./...` rather than a binary.
  { name: "shared-go", dir: join(repoRoot, "shared", "go"), eco: "go", lib: true },
  // Its TS twin: @bench/shared carries the same strict Biome ladder as the
  // servers, so its own code must be gated too — not just its config (which the
  // biome-sync check covers).
  { name: "shared-typescript", dir: join(repoRoot, "shared", "typescript"), eco: "pnpm" },
  // Its Python twin: bench-shared carries the same strict pyright + ruff ladder as
  // py-fastapi (its only consumer today), so its own code must be gated too — the
  // uv eco steps (pyright · ruff format --check · ruff check) run standalone here
  // via its own dev group.
  { name: "shared-python", dir: join(repoRoot, "shared", "python"), eco: "uv" },
  // Rust twin: the shared crate carries the same clippy pedantic floor as the
  // server, and clippy/rustfmt only lint the crate in cwd — a path dependency
  // is compiled but never linted, so the crate needs its own row.
  { name: "shared-rust", dir: join(repoRoot, "shared", "rust"), eco: "cargo" },
  { name: "root", dir: repoRoot, eco: "root" }
];

export const SERVERS: Server[] = [...discoverServers(), ...EXTRA_TARGETS];

// ── Docker DB-stack detection (shared by contract.mts and calibrate.mts) ──

// The compose file our databases stack is defined in — used to pick OUR stack
// when other compose projects on this host also run a postgres service.
export const dbComposeFile = join(repoRoot, "infra", "docker", "databases.yml");

// Case-insensitive path handling: macOS filesystems are case-insensitive and
// compose records whatever casing it was invoked from (e.g. ~/dev vs ~/Dev).
// The stack may have been created from ANY checkout of this repo (main clone or
// a .claude/worktrees agent worktree), so "ours" means: the recorded compose
// file is our databases.yml inside the main repo root (worktrees live under it).
const mainRepoRoot = (() => {
  const res = spawnSync("git", ["rev-parse", "--path-format=absolute", "--git-common-dir"], {
    cwd: repoRoot,
    encoding: "utf8"
  });
  return res.status === 0 ? dirname(res.stdout.trim()) : repoRoot;
})();

function isOurComposeFile(path: string): boolean {
  const p = resolve(path).toLowerCase();
  const suffix = join("infra", "docker", "databases.yml").toLowerCase();
  // The trailing separator is load-bearing: without it a sibling checkout
  // like <root>-backup would match too.
  return p.startsWith(resolve(mainRepoRoot).toLowerCase() + sep) && p.endsWith(suffix);
}

export type DbStack = { project: string; network: string };

// The server container reaches the DBs by compose service name (postgres,
// mongodb, ...), so it must join the network OUR DB containers live on.
// Other compose projects on this host may also run a postgres service (other
// repos, the Phase 2 metrics-postgres), so "first postgres container" is not
// safe: match on the com.docker.compose.project.config_files label pointing at
// our infra/docker/databases.yml, and fail loud on zero or ambiguous matches.
export function detectDbStack(): DbStack {
  const ps = spawnSync(
    "docker",
    ["ps", "--filter", "label=com.docker.compose.service=postgres", "--format", "{{.ID}}"],
    { encoding: "utf8" }
  );
  const ids = ps.stdout.trim().split("\n").filter(Boolean);
  if (ids.length === 0) {
    fatal("no running postgres container found — start the databases with:  just db-up");
  }

  type Candidate = { name: string; project: string; configFiles: string; network: string };
  const candidates: Candidate[] = [];
  for (const id of ids) {
    const inspect = spawnSync(
      "docker",
      [
        "inspect",
        id,
        "--format",
        `{{.Name}}\t{{index .Config.Labels "com.docker.compose.project"}}\t{{index .Config.Labels "com.docker.compose.project.config_files"}}\t{{range $k,$v := .NetworkSettings.Networks}}{{$k}} {{end}}`
      ],
      { encoding: "utf8" }
    );
    // Trim only the trailing newline: .trim() would eat the tab before an
    // empty networks field and shift the destructuring.
    const [name = "", project = "", configFiles = "", network = ""] = inspect.stdout
      .replace(/\n+$/, "")
      .split("\t");
    candidates.push({ name: name.replace(/^\//, ""), project, configFiles, network });
  }

  // config_files is a comma-separated list; ours is a single file.
  const ours = candidates.filter((c) => c.configFiles.split(",").some((f) => isOurComposeFile(f.trim())));

  if (ours.length === 0) {
    const list = candidates.map((c) => `    ${c.name} (project=${c.project}, config=${c.configFiles})`).join("\n");
    fatal(
      `found running postgres container(s), but none belong to this repo's databases stack (${dbComposeFile}):\n${list}\n  Start ours with:  just db-up`
    );
  }
  if (ours.length > 1) {
    const list = ours.map((c) => `    ${c.name} (project=${c.project}, network=${c.network})`).join("\n");
    fatal(`multiple postgres containers match ${dbComposeFile} — ambiguous stack, stop the extras:\n${list}`);
  }
  const stack = ours[0];
  // A container attached to extra networks (docker network connect) still
  // belongs to its project's default network; anything else is ambiguous.
  const networks = stack.network.split(/\s+/).filter(Boolean);
  let network = networks[0] ?? "";
  if (networks.length > 1) {
    const def = `${stack.project}_default`;
    if (!networks.includes(def)) {
      fatal(`container ${stack.name} is attached to multiple networks (${networks.join(", ")}) and none is ${def}`);
    }
    network = def;
  }
  if (!network) fatal(`could not detect the docker network for container ${stack.name}`);
  return { project: stack.project, network };
}

// Preflight the SAME stack whose network the server will join: every database
// service must have a running, healthy container in that compose project.
export function dbPreflight(stack: DbStack, databases: string[]): void {
  const bad: string[] = [];
  for (const db of databases) {
    const ps = spawnSync(
      "docker",
      [
        "ps",
        "--filter",
        `label=com.docker.compose.project=${stack.project}`,
        "--filter",
        `label=com.docker.compose.service=${db}`,
        "--format",
        "{{.ID}}"
      ],
      { encoding: "utf8" }
    );
    const id = ps.stdout.trim().split("\n")[0]?.trim();
    if (!id) {
      bad.push(`${db} (no running container in project ${stack.project})`);
      continue;
    }
    const health = spawnSync("docker", ["inspect", id, "--format", "{{.State.Health.Status}}"], {
      encoding: "utf8"
    }).stdout.trim();
    if (health !== "healthy") bad.push(`${db} (health: ${health || "unknown"})`);
  }
  if (bad.length > 0) {
    fatal(`databases not ready: ${bad.join(", ")}\n  Start them with:  just db-up`);
  }
}

export const c = {
  red: (s: string): string => `\x1b[31m${s}\x1b[0m`,
  green: (s: string): string => `\x1b[32m${s}\x1b[0m`,
  yellow: (s: string): string => `\x1b[33m${s}\x1b[0m`,
  cyan: (s: string): string => `\x1b[36m${s}\x1b[0m`,
  dim: (s: string): string => `\x1b[2m${s}\x1b[0m`,
  bold: (s: string): string => `\x1b[1m${s}\x1b[0m`
};

export type Step = { label: string; cmd: string; cwd: string };
export type Job = { name: string; steps: Step[] };
export type JobResult = { name: string; ok: boolean; ms: number; log: string; failed?: string };

function runStep(step: Step): Promise<{ ok: boolean; out: string }> {
  return new Promise((res) => {
    // shell:true so `&&` chains and `pnpm run` / `deno task` resolve normally.
    const child = spawn(step.cmd, { cwd: step.cwd, shell: true });
    let out = "";
    const append = (d: Buffer): void => {
      out += d.toString();
    };
    child.stdout.on("data", append);
    child.stderr.on("data", append);
    child.on("error", (e) => res({ ok: false, out: `${out}\n[spawn error] ${e.message}` }));
    child.on("close", (code) => res({ ok: code === 0, out }));
  });
}

async function runJob(job: Job): Promise<JobResult> {
  const start = Date.now();
  let log = "";
  for (const step of job.steps) {
    const { ok, out } = await runStep(step);
    log += `${c.dim(`$ ${step.cmd}  (${relative(repoRoot, step.cwd) || "."})`)}\n${out.trimEnd()}\n`;
    if (!ok) return { name: job.name, ok: false, ms: Date.now() - start, log, failed: step.label };
  }
  return { name: job.name, ok: true, ms: Date.now() - start, log };
}

// Bounded worker pool: at most `concurrency` jobs run at once.
export async function runJobs(jobs: Job[], concurrency = 5): Promise<JobResult[]> {
  const results: JobResult[] = new Array(jobs.length);
  let next = 0;
  const worker = async (): Promise<void> => {
    while (true) {
      const idx = next++;
      if (idx >= jobs.length) return;
      const job = jobs[idx];
      console.log(c.cyan(`› ${job.name}`));
      const r = await runJob(job);
      results[idx] = r;
      const t = `${(r.ms / 1000).toFixed(1)}s`;
      console.log(r.ok ? c.green(`✓ ${r.name} ${c.dim(t)}`) : c.red(`✗ ${r.name} (${r.failed}) ${c.dim(t)}`));
    }
  };
  await Promise.all(Array.from({ length: Math.min(concurrency, jobs.length) }, worker));
  return results;
}

// Grouped report: dump the full log of each failure, then a status table, then exit.
export function report(title: string, results: JobResult[]): never {
  const failures = results.filter((r) => !r.ok);
  for (const r of failures) {
    console.log(`\n${c.bold(c.red(`━━ ${r.name} failed (${r.failed}) ━━`))}`);
    console.log(r.log.trimEnd());
  }
  console.log(`\n${c.bold(`━━ ${title} ━━`)}`);
  const w = Math.max(4, ...results.map((r) => r.name.length));
  for (const r of results) {
    const status = r.ok ? c.green("PASS") : c.red("FAIL");
    const note = r.ok ? "" : c.dim(r.failed ?? "");
    console.log(`  ${r.name.padEnd(w)}  ${status}  ${note}`);
  }
  const n = failures.length;
  console.log(n ? c.red(`\n${n}/${results.length} failed`) : c.green(`\nall ${results.length} passed`));
  process.exit(n ? 1 : 0);
}

// Resolve a CLI target ("all" or a single name/image) against the eligible rows.
export function pickTargets(arg: string | undefined, eligible: Server[], label: string): Server[] {
  const target = arg ?? "all";
  if (target === "all") return eligible;
  const match = eligible.find((s) => s.name === target || s.image === target);
  if (!match) {
    const names = eligible.map((s) => s.name).join(", ");
    console.error(c.red(`unknown ${label} target "${target}". Known: ${names}, all`));
    process.exit(1);
  }
  return [match];
}

// First non-flag argv token (the target); defaults to "all".
export function targetArg(): string {
  return process.argv.slice(2).find((a) => !a.startsWith("-")) ?? "all";
}

export { spawnSync };
