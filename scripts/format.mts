// Write-formatting (Phase 0B): `just fmt <target|all>` — the mutating counterpart
// to verify's format CHECK. Mirrors the old justfile `fmt` recipe exactly.
//
//   node scripts/format.mts <target|all>

import { type Job, pickTargets, report, runJobs, SERVERS, type Server, targetArg } from "./lib.mts";

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
    case "root":
      return "pnpm run format"; // prettier --write
  }
}

const targets = pickTargets(targetArg(), SERVERS, "format");
const jobs: Job[] = targets.map((s) => ({ name: s.name, steps: [{ label: "format", cmd: formatCmd(s), cwd: s.dir }] }));
const results = await runJobs(jobs);
report("format", results);
