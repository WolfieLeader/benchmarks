// Pin-aware dependency update (Phase 0B, PLAN §10): `just update <target|all>`.
//
//   node scripts/update.mts <target|all>
//
// Blanket "update to latest" resolves the `latest` dist-tag and would clobber
// deliberate prerelease pins (typescript@7.0.1-rc, drizzle 1.0-rc) and lockstep
// sets (elysia + @elysiajs/node). Packages listed in PINNED are exempt from the
// blanket auto-bump but are NOT frozen: each pin tracks a dist-tag channel and
// is re-resolved against the registry on every run, so `just update` moves a
// pin to the newest release of its channel (e.g. rc.4 -> rc.5) while never
// letting `--latest` drag it back to the stable line.

import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { c, type Job, pickTargets, report, runJobs, SERVERS, type Server, spawnSync, targetArg } from "./lib.mts";

// Prerelease pins: pkg -> npm dist-tag channel to track. `dev` controls where
// the npm-family re-pin lands (dependencies vs devDependencies). A pin only
// applies to a target whose manifest already declares the package — update
// never introduces a dependency.
type Pin = { pkg: string; channel: string; dev?: boolean; reason: string };
const PINNED: Pin[] = [
  { pkg: "typescript", channel: "rc", dev: true, reason: "TypeScript 7 native tsc RC (PLAN §10); `latest` is still 6.x" },
  { pkg: "drizzle-orm", channel: "rc", reason: "drizzle 1.0 RC (PLAN §10); `latest` is still 0.45.x" }
  // Lockstep sets (elysia + @elysiajs/node) get a row here when the second
  // half of the set actually lands (0E); today @elysiajs/node is not a dep.
];

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

// Resolve every pinned channel lazily and only once. Only the npm-family
// updaters (pnpm/bun/deno) consult the registry, so a Go- or uv-only selection
// never shells `npm view` (and never hard-exits offline). Fails loud via
// resolveChannel the moment resolution IS needed.
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

// Deno pins live in deno.json `imports` as `npm:<pkg>@<version>` specifiers,
// possibly across several subpath aliases (drizzle-orm, drizzle-orm/pg-core, …).
// `deno update --latest` follows the `latest` dist-tag, which for a prerelease
// pin is a downgrade — so pinned aliases are excluded via negative filters and
// the pinned version is rewritten directly in deno.json (manifest, not lockfile),
// then `deno install` refreshes the lock.
function refreshDenoPins(dir: string): Pin[] {
  const file = join(dir, "deno.json");
  const manifest = JSON.parse(readFileSync(file, "utf8")) as { imports?: Record<string, string> };
  const imports = manifest.imports ?? {};
  const applied: Pin[] = [];
  let changed = false;
  const resolved = resolvePins();
  for (const pin of PINNED) {
    const version = resolved.get(pin.pkg);
    const re = new RegExp(`^(npm:${pin.pkg})@[^/]+(/.*)?$`);
    let hit = false;
    for (const [alias, spec] of Object.entries(imports)) {
      const m = spec.match(re);
      if (!m) continue;
      hit = true;
      const next = `${m[1]}@${version}${m[2] ?? ""}`;
      if (next !== spec) {
        imports[alias] = next;
        changed = true;
      }
    }
    if (hit) applied.push(pin);
  }
  if (changed) writeFileSync(file, `${JSON.stringify({ ...manifest, imports }, null, 2)}\n`);
  return applied;
}

function updateCmd(s: Server): string {
  switch (s.eco) {
    case "pnpm":
    case "root": {
      const pins = pinsInPackageJson(s.dir);
      // pnpm supports `!pkg` negation directly in `update --latest`.
      const excludes = pins.map((p) => `"!${p.pkg}"`).join(" ");
      const bump = `pnpm update --latest${excludes ? ` ${excludes}` : ""}`;
      const resolved = pins.length > 0 ? resolvePins() : null;
      const repin = pins.map((p) => `pnpm add --save-exact ${p.dev ? "-D " : ""}${p.pkg}@${resolved?.get(p.pkg)}`);
      return [bump, ...repin, "pnpm install"].join(" && ");
    }
    case "bun": {
      // bun update has no negation filter: blanket-bump first, then re-pin each
      // pinned package at its freshly resolved channel version.
      const pins = pinsInPackageJson(s.dir);
      const resolved = pins.length > 0 ? resolvePins() : null;
      const repin = pins.map((p) => `bun add --exact ${p.dev ? "--dev " : ""}${p.pkg}@${resolved?.get(p.pkg)}`);
      return ["bun update --latest", ...repin, "bun install"].join(" && ");
    }
    case "deno": {
      // Manifest rewrite happens up front (main thread); the job only has to
      // skip pinned aliases during the blanket bump and refresh the lockfile.
      const pins = refreshDenoPins(s.dir);
      const excludes = pins.map((p) => `"!${p.pkg}*"`).join(" ");
      return `deno update --latest${excludes ? ` ${excludes}` : ""} && deno install`;
    }
    case "go":
      // Go pins are toolchain-level (go1.27rc1 via `toolchain`), not `go get -u`
      // targets, so no dep-level exclusion applies here.
      return "go get -u ./... && go mod tidy";
    case "uv":
      return "uv sync --upgrade && uv sync";
    case "zig":
      // Zig deps are commit-pinned in build.zig.zon (like the Go toolchain pin);
      // a blanket update leaves them, so this just re-resolves the pins.
      return "zig build --fetch";
  }
}

function printPins(): void {
  if (PINNED.length === 0) {
    console.log(c.dim("pinned deps (exempt from blanket bump): none"));
    return;
  }
  console.log(c.bold("pinned deps (exempt from blanket bump; tracking their channel):"));
  for (const p of PINNED) {
    // Show the resolved version only if the selected targets already forced
    // resolution; a Go/uv-only run prints the channel without hitting npm.
    const v = pinVersions?.get(p.pkg);
    console.log(`  ${c.yellow(p.pkg)}@${p.channel}${v ? ` -> ${v}` : ""} — ${p.reason}`);
  }
}

const targets = pickTargets(targetArg(), SERVERS, "update");
// Building the jobs runs updateCmd per target, which lazily resolves pins for
// any npm-family target; printPins then reports whatever was resolved.
const jobs: Job[] = targets.map((s) => ({ name: s.name, steps: [{ label: "update", cmd: updateCmd(s), cwd: s.dir }] }));
printPins();
const results = await runJobs(jobs);
report("update", results);
