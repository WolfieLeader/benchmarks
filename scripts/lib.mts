// Shared helpers for the scripts/ orchestrators (Phase 0B).
//
// One roster table (SERVERS) is the single source of truth for the target list;
// each script maps its rows to concrete shell commands. A bounded-concurrency
// runner executes per-target jobs in parallel and a grouped report prints at the
// end instead of bailing on the first failure (`a && b && c` hides later errors).
//
// Run with Node 26 native type-stripping — erasable-syntax TS only, no build step.

import { spawn, spawnSync } from "node:child_process";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";

export const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..");

const ts = (p: string): string => join(repoRoot, "http-servers", "typescript", p);
const go = (p: string): string => join(repoRoot, "http-servers", "go", p);
const py = (p: string): string => join(repoRoot, "http-servers", "python", p);

export type Eco = "pnpm" | "bun" | "deno" | "uv" | "go" | "root";

export type Server = {
  name: string; // CLI target key (e.g. "express", "chi")
  dir: string; // working directory for its commands
  eco: Eco; // toolchain family — commands derive from this per script
  image?: string; // docker image tag; present => included in `images`
  dev?: string; // dev command; present => included in `dev`
  goBin?: string; // eco "go": build output name (default "server")
};

// The roster — add a target = add one row. Order here is the report order.
export const SERVERS: Server[] = [
  { name: "express", dir: ts("node-express"), eco: "pnpm", image: "node-express", dev: "pnpm run dev" },
  { name: "fastify", dir: ts("node-fastify"), eco: "pnpm", image: "node-fastify", dev: "pnpm run dev" },
  { name: "nestjs", dir: ts("node-nestjs"), eco: "pnpm", image: "node-nestjs", dev: "pnpm run dev" },
  { name: "honojs", dir: ts("bun-honojs"), eco: "bun", image: "bun-honojs", dev: "bun run dev" },
  { name: "elysia", dir: ts("bun-elysia"), eco: "bun", image: "bun-elysia", dev: "bun run dev" },
  { name: "oak", dir: ts("deno-oak"), eco: "deno", image: "deno-oak", dev: "deno task dev" },
  { name: "chi", dir: go("chi"), eco: "go", image: "go-chi", dev: "air" },
  { name: "gin", dir: go("gin"), eco: "go", image: "go-gin", dev: "air" },
  { name: "fiber", dir: go("fiber"), eco: "go", image: "go-fiber", dev: "air" },
  { name: "fastapi", dir: py("fastapi"), eco: "uv", image: "python-fastapi", dev: "uv run python -m src.main" },
  { name: "benchmark", dir: join(repoRoot, "benchmarks"), eco: "go", goBin: "benchmark" },
  { name: "root", dir: repoRoot, eco: "root" }
];

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
