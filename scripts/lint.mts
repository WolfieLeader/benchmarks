// Lint (Phase 0B): `just lint <target|all>`. Preserves the old justfile `lint`
// recipe semantics per tool — TS/Deno auto-fix (biome check --fix / deno lint
// --fix), Go/Python/root check-only (golangci-lint run / ruff check / prettier
// --check). The non-mutating lint used by the merge gate lives in verify.mts.
//
//   node scripts/lint.mts <target|all>

import { gradleGroup, gradlew, type Job, pickTargets, repoRoot, report, runJobs, SERVERS, type Server, targetArg } from "./lib.mts";

function lintCmd(s: Server): string {
  switch (s.eco) {
    case "pnpm":
      return "pnpm run lint:fix"; // biome check --fix ./src
    case "bun":
      return "bun run lint:fix"; // biome check --fix ./src
    case "deno":
      return "deno task lint:fix"; // deno lint --fix
    case "go":
      // --allow-parallel-runners: skip the machine-wide lock that kills
      // concurrent `run`s from our worker pool (see verify.mts for details).
      return "golangci-lint run --allow-parallel-runners ./...";
    case "uv":
      return "uv run ruff check .";
    case "zig":
      return "zig build"; // the compiler is the linter (PLAN §3)
    case "cargo":
      return "cargo clippy -- -D warnings"; // check-only, like the Go/Python linters
    case "gradle":
      return ""; // collapsed into one repo-root job below; never reached per-target
    case "root":
      return "pnpm run lint"; // prettier --check
  }
}

const targets = pickTargets(targetArg(), SERVERS, "lint");
const jobs: Job[] = targets
  .filter((s) => s.eco !== "gradle")
  .map((s) => ({ name: s.name, steps: [{ label: "lint", cmd: lintCmd(s), cwd: s.dir }] }));

// detekt is the Kotlin linter (check-only, like clippy/ruff); one gradlew
// invocation from repoRoot covers every in-scope Kotlin project (build-lock).
const grp = gradleGroup(targets);
if (grp) {
  const tasks = grp.projects.map((p) => `${p}:detekt`).join(" ");
  jobs.push({ name: grp.name, steps: [{ label: "lint", cmd: `${gradlew} ${tasks}`, cwd: repoRoot }] });
}

const results = await runJobs(jobs);
report("lint", results);
