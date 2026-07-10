// Write-formatting (Phase 0B): `just fmt <target|all>` — the mutating counterpart
// to verify's format CHECK. Mirrors the old justfile `fmt` recipe exactly.
//
//   node scripts/format.mts <target|all>

import { gradleGroup, gradlew, type Job, pickTargets, repoRoot, report, runJobs, SERVERS, type Server, targetArg } from "./lib.mts";

function formatCmd(s: Server): string {
  switch (s.eco) {
    case "pnpm":
      return "pnpm run format"; // biome format --write ./src
    case "bun":
      return "bun run format"; // biome format --write ./src
    case "deno":
      return "deno task format"; // deno fmt
    case "go":
      return "golangci-lint fmt ./...";
    case "uv":
      return "uv run ruff format .";
    case "zig":
      return "zig fmt src build.zig";
    case "cargo":
      return "cargo fmt";
    case "gradle":
      return ""; // collapsed into one repo-root job below; never reached per-target
    case "root":
      return "pnpm run format"; // prettier --write
  }
}

const targets = pickTargets(targetArg(), SERVERS, "format");
const jobs: Job[] = targets
  .filter((s) => s.eco !== "gradle")
  .map((s) => ({ name: s.name, steps: [{ label: "format", cmd: formatCmd(s), cwd: s.dir }] }));

// ktlintFormat is the write counterpart to verify's ktlintCheck; one gradlew
// invocation from repoRoot covers every in-scope Kotlin project (build-lock).
const grp = gradleGroup(targets);
if (grp) {
  const tasks = grp.projects.map((p) => `${p}:ktlintFormat`).join(" ");
  jobs.push({ name: grp.name, steps: [{ label: "format", cmd: `${gradlew} ${tasks}`, cwd: repoRoot }] });
}

const results = await runJobs(jobs);
report("format", results);
