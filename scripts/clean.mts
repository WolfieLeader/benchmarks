// Remove build artifacts and dependency dirs for every roster target
// (`just clean`). Manifest-driven: the target list and each target's toolchain
// family come from scripts/lib.mts's SERVERS (servers/*/bench.json + the two
// extra rows), so adding a server needs no edit here — no hardcoded list.
//
//   node scripts/clean.mts

import { rmSync } from "node:fs";
import { join, relative } from "node:path";
import { c, repoRoot, SERVERS } from "./lib.mts";

// Per-toolchain-family artifact/dependency dirs, relative to each target's dir.
// Families with nothing to clean (e.g. "root") are simply absent.
const ARTIFACTS: Partial<Record<string, string[]>> = {
  pnpm: ["node_modules"],
  bun: ["node_modules"],
  deno: ["node_modules"],
  go: ["bin", "tmp"],
  uv: [".venv", "__pycache__", join("src", "__pycache__")],
  zig: [".zig-cache", "zig-out", "zig-pkg"]
};

console.log(c.cyan("› cleaning build artifacts"));
for (const s of SERVERS) {
  const dirs = ARTIFACTS[s.eco];
  if (!dirs) continue;
  for (const d of dirs) {
    const target = join(s.dir, d);
    rmSync(target, { recursive: true, force: true });
    console.log(c.dim(`  rm ${relative(repoRoot, target)}`));
  }
}
console.log(c.green("✓ clean complete"));
