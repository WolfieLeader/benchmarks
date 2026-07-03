// Pin-aware dependency update (Phase 0B, PLAN §10): `just update <target|all>`.
//
//   node scripts/update.mts <target|all>
//
// Blanket "update to latest" resolves the `latest` dist-tag and would clobber
// deliberate prerelease pins (typescript@7.0.1-rc, drizzle 1.0-rc) and lockstep
// sets (elysia + @elysiajs/node). Packages listed in PINNED are exempt from the
// auto-bump and reported separately. The list is EMPTY today — the RC pins land
// in Phase 0D — but the mechanism is wired so adding a pin is a one-line edit.

import { c, type Job, pickTargets, report, runJobs, SERVERS, type Server, targetArg } from "./lib.mts";

// Prerelease / lockstep deps to skip during `--latest` auto-bump. Empty today.
// When 0D lands the RC pins, add rows like:
//   { pkg: "typescript",     reason: "7.0.1-rc; `latest` is still 6.x — a blanket bump clobbers the RC pin" },
//   { pkg: "drizzle-orm",    reason: "1.0-rc pin" },
//   { pkg: "drizzle-kit",    reason: "1.0-rc pin" },
//   { pkg: "@elysiajs/node", reason: "must move in lockstep with elysia" },
const PINNED: { pkg: string; reason: string }[] = [];

// Exclusion tokens for the npm-family updaters. pnpm's update accepts `!pattern`
// negation; bun/deno exclusion syntax is unverified and must be re-checked when
// the first npm-family pin lands (0D). Empty PINNED => no tokens => base command.
function npmExcludes(): string {
  return PINNED.map((p) => `"!${p.pkg}"`).join(" ");
}

function updateCmd(s: Server): string {
  const ex = npmExcludes();
  const withEx = (base: string): string => (ex ? `${base} ${ex}` : base);
  switch (s.eco) {
    case "pnpm":
    case "root":
      return `${withEx("pnpm update --latest")} && pnpm install`;
    case "bun":
      return `${withEx("bun update --latest")} && bun install`;
    case "deno":
      return `${withEx("deno update --latest")} && deno install`;
    case "go":
      // Go pins are toolchain-level (go1.27rc1 via `toolchain`), not `go get -u`
      // targets, so no dep-level exclusion applies here.
      return "go get -u ./... && go mod tidy";
    case "uv":
      return "uv sync --upgrade && uv sync";
  }
}

function printPins(): void {
  if (PINNED.length === 0) {
    console.log(c.dim("pinned deps (exempt from auto-bump): none"));
    return;
  }
  console.log(c.bold("pinned deps (exempt from auto-bump):"));
  for (const p of PINNED) console.log(`  ${c.yellow(p.pkg)} — ${p.reason}`);
}

printPins();
const targets = pickTargets(targetArg(), SERVERS, "update");
const jobs: Job[] = targets.map((s) => ({ name: s.name, steps: [{ label: "update", cmd: updateCmd(s), cwd: s.dir }] }));
const results = await runJobs(jobs);
report("update", results);
