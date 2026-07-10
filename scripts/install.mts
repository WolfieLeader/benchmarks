// Dependency install (Phase 0B): `just install <target|all>`.
//
//   node scripts/install.mts <target|all>
//
// TypeScript is a single pnpm workspace (PLAN §2.2): the Node, Bun and Deno servers
// are all pnpm members (Bun/Deno consume the pnpm-installed node_modules; they no
// longer run their own package managers). So every TS target collapses to ONE
// `pnpm install` at the repo root — deduped into a single job to avoid concurrent
// installs racing on the shared store/lockfile — followed by building @bench/shared
// so its dist exists for downstream typechecks/runs. Go, Python and Zig install
// per-dir (Zig is not a pnpm member — `zig build --fetch` resolves build.zig.zon).

import { gradleGroup, gradlew, type Job, pickTargets, repoRoot, report, runJobs, SERVERS, targetArg } from "./lib.mts";

const targets = pickTargets(targetArg(), SERVERS, "install");
const jobs: Job[] = [];

// pnpm / bun / deno / root all belong to the one pnpm workspace.
if (targets.some((s) => s.eco === "pnpm" || s.eco === "bun" || s.eco === "deno" || s.eco === "root")) {
  jobs.push({
    name: "workspace",
    steps: [
      { label: "install", cmd: "pnpm install", cwd: repoRoot },
      { label: "build-shared", cmd: "pnpm --filter @bench/shared run build", cwd: repoRoot },
      // The hono servers typecheck against @bench/hono-app's dist, so a fresh
      // install must build it too (it consumes @bench/shared — keep this after).
      { label: "build-hono-app", cmd: "pnpm --filter @bench/hono-app run build", cwd: repoRoot }
    ]
  });
}

for (const s of targets) {
  if (s.eco === "go") jobs.push({ name: s.name, steps: [{ label: "install", cmd: "go mod tidy", cwd: s.dir }] });
  else if (s.eco === "uv") jobs.push({ name: s.name, steps: [{ label: "install", cmd: "uv sync", cwd: s.dir }] });
  // Zig deps live in build.zig.zon; `--fetch` resolves them without a full build.
  else if (s.eco === "zig") jobs.push({ name: s.name, steps: [{ label: "install", cmd: "zig build --fetch", cwd: s.dir }] });
  // Cargo deps resolve from Cargo.toml; `fetch` downloads them without building.
  else if (s.eco === "cargo") jobs.push({ name: s.name, steps: [{ label: "install", cmd: "cargo fetch", cwd: s.dir }] });
}

// Gradle: one invocation from repoRoot bootstraps the committed wrapper (downloads
// the pinned distribution) and resolves every in-scope project's dependencies into
// the Gradle cache. Collapsed to one job — concurrent gradlew calls contend on the
// build lock.
const grp = gradleGroup(targets);
if (grp) {
  const tasks = grp.projects.map((p) => `${p}:dependencies`).join(" ");
  jobs.push({ name: grp.name, steps: [{ label: "install", cmd: `${gradlew} ${tasks}`, cwd: repoRoot }] });
}

const results = await runJobs(jobs);
report("install", results);
