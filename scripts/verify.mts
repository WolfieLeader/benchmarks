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

import {
  type Job,
  pickTargets,
  repoRoot,
  report,
  runJobs,
  SERVERS,
  type Server,
  type Step,
  targetArg
} from "./lib.mts";

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
        // Library modules (shared/go) have no ./cmd/main.go — build every package.
        typecheck: st("typecheck", s.lib ? "go build ./..." : `go build -o bin/${s.goBin ?? "server"} ./cmd/main.go`),
        format: st("format", "golangci-lint fmt --diff ./..."),
        // --allow-parallel-runners: `run` (unlike `fmt`) takes a machine-wide
        // lock ($TMPDIR/golangci-lint.lock, ~5s grace) and then dies with
        // "parallel golangci-lint is running" — our worker pool runs the Go
        // targets concurrently, so without this flag verify-all flakes. Safe:
        // the analysis cache is a fork of cmd/go's build cache, documented
        // multi-process-safe on one machine ("may duplicate effort but will
        // not corrupt the cache"), and each target is a separate module dir.
        lint: st("lint", "golangci-lint run --allow-parallel-runners ./...")
      };
    case "uv":
      return {
        typecheck: st("typecheck", "uv run pyright src"),
        format: st("format", "uv run ruff format --check ."),
        lint: st("lint", "uv run ruff check .")
      };
    case "zig":
      // `zig build` compiles the whole program — it is both the type/build check
      // and the linter (PLAN §3: "compiler is the linter"). fmt scopes to our
      // sources so vendored deps (zig-pkg) and caches are not format-checked.
      return {
        typecheck: st("typecheck", "zig build"),
        format: st("format", "zig fmt --check src build.zig"),
        lint: null
      };
    case "cargo":
      // rustfmt stays at defaults (PLAN §3); clippy runs with `-D warnings` over
      // the pedantic floor (per-crate `[lints]`), so a lone warn fails the gate.
      return {
        typecheck: st("typecheck", "cargo build"),
        format: st("format", "cargo fmt --check"),
        lint: st("lint", "cargo clippy -- -D warnings")
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

// Config/manifest drift is a repo-wide gate, not a per-target check: run it as its
// own row whenever the root target is in scope (so `just verify` / `just verify root`
// fail on drift), and skip it when --only narrows to a single per-target check kind.
if (!only && targets.some((s) => s.eco === "root")) {
  jobs.push({
    name: "check-config",
    steps: [{ label: "check-config", cmd: "node scripts/check-config.mts", cwd: repoRoot }]
  });
  // The per-project full-copy biome.jsonc files (Go-style, no root config) must
  // not silently drift apart — this repo-wide gate structurally compares them
  // against an allowlist of known per-project deviations. Runs alongside
  // check-config whenever the root target is in scope.
  jobs.push({
    name: "biome-sync",
    steps: [{ label: "biome-sync", cmd: "node scripts/biome-sync-check.mts", cwd: repoRoot }]
  });
  // Same guard for the per-module full-copy .golangci.json files (no shared base):
  // structurally compare every Go module's ladder against a reference, allowing
  // only the known per-module deviations (gofumpt module-path, forbidigo scoping).
  jobs.push({
    name: "golangci-sync",
    steps: [{ label: "golangci-sync", cmd: "node scripts/golangci-sync-check.mts", cwd: repoRoot }]
  });
  // Dead exports/deps across the workspace (knip.json holds the rationale for
  // why this runs at the root: cross-package visibility into @bench/shared).
  jobs.push({
    name: "knip",
    steps: [{ label: "knip", cmd: "pnpm exec knip", cwd: repoRoot }]
  });
}

const results = await runJobs(jobs);
report(only ? `verify --only=${only}` : "verify", results);
