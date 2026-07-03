// Pin-aware dependency update (Phase 0B, PLAN §10): `just update <target|all>`.
//
//   node scripts/update.mts <target|all>
//
// Blanket "update to latest" resolves the `latest` dist-tag and would clobber
// deliberate prerelease pins (typescript@7.0.1-rc, drizzle 1.0-rc). Packages listed
// in PINNED are exempt from the blanket auto-bump but are NOT frozen: each pin
// tracks a dist-tag channel and is re-resolved against the registry on every run,
// so `just update` moves a pin to the newest release of its channel (e.g. rc.4 ->
// rc.5) while never letting `--latest` drag it back to the stable line.
//
// TypeScript is a single pnpm workspace (PLAN §2.2): the Node, Bun and Deno servers
// are all pnpm members (no per-server bun/deno package managers). So a TS/root
// target updates the WHOLE workspace in one job — `pnpm -r update --latest`
// (recursive across every member incl. shared/typescript) with the pins excluded,
// then re-pin each pin in the members that declare it. Go/Python/Zig update per-dir
// (Zig is not a pnpm member — `zig build --fetch` re-resolves its pinned deps).

import { readFileSync } from "node:fs";
import { join, relative } from "node:path";
import {
  c,
  type Job,
  pickTargets,
  repoRoot,
  report,
  runJobs,
  SERVERS,
  type Step,
  spawnSync,
  targetArg
} from "./lib.mts";

// Prerelease pins: pkg -> npm dist-tag channel to track. `dev` controls where the
// re-pin lands (dependencies vs devDependencies). A pin only applies to a member
// whose manifest already declares the package — update never introduces a dep.
type Pin = { pkg: string; channel: string; dev?: boolean; reason: string };
const PINNED: Pin[] = [
  { pkg: "typescript", channel: "rc", dev: true, reason: "TypeScript 7 native tsc RC (PLAN §10); `latest` is still 6.x" },
  { pkg: "drizzle-orm", channel: "rc", reason: "drizzle 1.0 RC (PLAN §10); `latest` is still 0.45.x" }
];

// Held at a non-latest range for compatibility, so excluded from `--latest` (not a
// channel pin): path-to-regexp is @oak/oak's only npm transitive and must stay on
// the 6.x line oak expects (latest is 8.x) — see servers/ts-deno-oak/package.json.
const HELD_BACK = ["path-to-regexp"];

// Resolve a pinned channel to its current version. Fails loud: a silently
// unresolved pin would let the blanket bump clobber it.
function resolveChannel(pkg: string, channel: string): string {
  const res = spawnSync("npm", ["view", pkg, `dist-tags.${channel}`], { encoding: "utf8" });
  const version = res.status === 0 ? res.stdout.trim() : "";
  if (!version) {
    console.error(c.red(`failed to resolve ${pkg}@${channel} against the npm registry`));
    process.exit(1);
  }
  return version;
}

let pinVersions: Map<string, string> | null = null;
function resolvePins(): Map<string, string> {
  if (pinVersions === null) pinVersions = new Map(PINNED.map((p) => [p.pkg, resolveChannel(p.pkg, p.channel)]));
  return pinVersions;
}

// Pins that apply to a given npm-family manifest (package.json).
function pinsInPackageJson(dir: string): Pin[] {
  const m = JSON.parse(readFileSync(join(dir, "package.json"), "utf8")) as {
    dependencies?: Record<string, string>;
    devDependencies?: Record<string, string>;
  };
  return PINNED.filter((p) => m.dependencies?.[p.pkg] !== undefined || m.devDependencies?.[p.pkg] !== undefined);
}

// The TS workspace members that carry an npm manifest: the pnpm/bun/deno servers
// (all pnpm members) plus shared/typescript (not a bench.json server, so appended).
function tsWorkspaceDirs(): string[] {
  const serverDirs = SERVERS.filter((s) => ["pnpm", "bun", "deno"].includes(s.eco)).map((s) => s.dir);
  return [...serverDirs, join(repoRoot, "shared", "typescript")];
}

// One job that updates the entire TS pnpm workspace and re-applies the pins.
function tsWorkspaceSteps(): Step[] {
  const root = repoRoot;
  const dirs = tsWorkspaceDirs();
  const pinsPresent = PINNED.filter((p) => dirs.some((d) => pinsInPackageJson(d).some((q) => q.pkg === p.pkg)));
  const resolved = pinsPresent.length > 0 ? resolvePins() : null;
  const excludes = [...pinsPresent.map((p) => p.pkg), ...HELD_BACK].map((n) => `"!${n}"`).join(" ");

  const steps: Step[] = [{ label: "bump", cmd: `pnpm -r update --latest ${excludes}`.trim(), cwd: root }];
  for (const dir of dirs) {
    const rel = relative(root, dir);
    for (const p of pinsInPackageJson(dir)) {
      steps.push({
        label: `pin ${p.pkg} @ ${rel}`,
        cmd: `pnpm --filter "./${rel}" add --save-exact ${p.dev ? "-D " : ""}${p.pkg}@${resolved?.get(p.pkg)}`,
        cwd: root
      });
    }
  }
  steps.push({ label: "install", cmd: "pnpm install", cwd: root });
  return steps;
}

function printPins(): void {
  console.log(c.bold("pinned deps (exempt from blanket bump; tracking their channel):"));
  for (const p of PINNED) {
    const v = pinVersions?.get(p.pkg);
    console.log(`  ${c.yellow(p.pkg)}@${p.channel}${v ? ` -> ${v}` : ""} — ${p.reason}`);
  }
  console.log(c.dim("  held back (excluded from --latest): " + HELD_BACK.join(", ")));
  // pnpm 11's minimumReleaseAge (1-day cooldown) can reject a re-pin whose freshly
  // published RC is <24h old: `npm view` still resolves it but `pnpm add` refuses
  // with ERR_PNPM_MINIMUM_RELEASE_AGE_VIOLATION. That failure is expected — wait a
  // day or add a minimumReleaseAgeExclude entry in pnpm-workspace.yaml.
  console.log(c.dim("  (a re-pin may fail if its RC is <24h old — pnpm's release cooldown; see pnpm-workspace.yaml)"));
}

const targets = pickTargets(targetArg(), SERVERS, "update");
const jobs: Job[] = [];
if (targets.some((s) => ["pnpm", "bun", "deno", "root"].includes(s.eco))) {
  jobs.push({ name: "workspace", steps: tsWorkspaceSteps() });
}
for (const s of targets) {
  if (s.eco === "go") jobs.push({ name: s.name, steps: [{ label: "update", cmd: "go get -u ./... && go mod tidy", cwd: s.dir }] });
  else if (s.eco === "uv") jobs.push({ name: s.name, steps: [{ label: "update", cmd: "uv sync --upgrade && uv sync", cwd: s.dir }] });
  // Zig deps are commit-pinned in build.zig.zon (like the Go toolchain pin); a blanket
  // update leaves them, so this just re-resolves the pins via `zig build --fetch`.
  else if (s.eco === "zig") jobs.push({ name: s.name, steps: [{ label: "update", cmd: "zig build --fetch", cwd: s.dir }] });
}
printPins();
const results = await runJobs(jobs);
report("update", results);
