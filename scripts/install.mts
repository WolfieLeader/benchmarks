// Dependency install (Phase 0B): `just install <target|all>`. Mirrors the old
// justfile `install` recipe per ecosystem.
//
//   node scripts/install.mts <target|all>

import { type Job, pickTargets, report, runJobs, SERVERS, type Server, targetArg } from "./lib.mts";

function installCmd(s: Server): string {
  switch (s.eco) {
    case "pnpm":
    case "root":
      return "pnpm install";
    case "bun":
      return "bun install";
    case "deno":
      return "deno install";
    case "go":
      return "go mod tidy";
    case "uv":
      return "uv sync";
  }
}

const targets = pickTargets(targetArg(), SERVERS, "install");
const jobs: Job[] = targets.map((s) => ({
  name: s.name,
  steps: [{ label: "install", cmd: installCmd(s), cwd: s.dir }]
}));
const results = await runJobs(jobs);
report("install", results);
