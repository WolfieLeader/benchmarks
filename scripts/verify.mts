// Non-mutating verification gate (Phase 0B): type/build check + format CHECK + lint.
//
//   node scripts/verify.mts <target|all> [--only=typecheck|format|lint]
//
// `just verify` must never write to the tree — write-formatting lives in
// scripts/format.mts (`just fmt`). Each check preserves the tool invocation the
// old justfile used, switched to its non-mutating variant:
//   pnpm/bun  → tsc --noEmit · biome format (no --write) · biome lint
//   deno      → deno check   · deno fmt --check          · deno lint
//   go        → go build     · golangci-lint fmt --diff  · golangci-lint run
//   uv/python → pyright      · ruff format --check       · ruff check
//   root      → (none)       · prettier --check          · (none)

import { type Job, pickTargets, report, runJobs, SERVERS, type Server, type Step, targetArg } from "./lib.mts";

type CheckKind = "typecheck" | "format" | "lint";
const ORDER: CheckKind[] = ["typecheck", "format", "lint"];

function checks(s: Server): Record<CheckKind, Step | null> {
  const cwd = s.dir;
  const st = (label: CheckKind, cmd: string): Step => ({ label, cmd, cwd });
  switch (s.eco) {
    case "pnpm":
      return {
        typecheck: st("typecheck", "pnpm run typecheck"),
        format: st("format", "pnpm exec biome format ./src"),
        lint: st("lint", "pnpm run lint")
      };
    case "bun":
      return {
        typecheck: st("typecheck", "bun run typecheck"),
        format: st("format", "bunx biome format ./src"),
        lint: st("lint", "bun run lint")
      };
    case "deno":
      return {
        typecheck: st("typecheck", "deno task check"),
        format: st("format", "deno fmt --check"),
        lint: st("lint", "deno task lint")
      };
    case "go":
      return {
        typecheck: st("typecheck", `go build -o bin/${s.goBin ?? "server"} ./cmd/main.go`),
        format: st("format", "golangci-lint fmt --diff ./..."),
        lint: st("lint", "golangci-lint run ./...")
      };
    case "uv":
      return {
        typecheck: st("typecheck", "uv run pyright src"),
        format: st("format", "uv run ruff format --check ."),
        lint: st("lint", "uv run ruff check .")
      };
    case "root":
      // Root only carries prettier; its `lint` script IS the format check.
      return { typecheck: null, format: st("format", "pnpm run lint"), lint: null };
  }
}

function stepsFor(s: Server, only: CheckKind | null): Step[] {
  const all = checks(s);
  const steps: Step[] = [];
  for (const kind of ORDER) {
    if (only && kind !== only) continue;
    const step = all[kind];
    if (step) steps.push(step);
  }
  return steps;
}

const onlyArg = process.argv.slice(2).find((a) => a.startsWith("--only="));
const only = (onlyArg ? onlyArg.slice("--only=".length) : null) as CheckKind | null;
if (only && !ORDER.includes(only)) {
  console.error(`--only must be one of: ${ORDER.join(", ")}`);
  process.exit(1);
}

const targets = pickTargets(targetArg(), SERVERS, "verify");
const jobs: Job[] = targets.map((s) => ({ name: s.name, steps: stepsFor(s, only) })).filter((j) => j.steps.length > 0);
const results = await runJobs(jobs);
report(only ? `verify --only=${only}` : "verify", results);
