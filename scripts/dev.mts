// Start one dev server in the foreground (Phase 0B): `just dev <server>`. This is
// interactive/long-running, so it is NOT concurrent or grouped — it execs the
// server's dev command and forwards its exit code (Ctrl-C reaches the child).
//
//   node scripts/dev.mts <server>

import { c, SERVERS, spawnSync } from "./lib.mts";

const runnable = SERVERS.filter((s) => s.dev);
const arg = process.argv.slice(2).find((a) => !a.startsWith("-"));

if (!arg || arg === "all") {
  console.error(c.red(`dev needs exactly one server. Known: ${runnable.map((s) => s.name).join(", ")}`));
  process.exit(1);
}

const server = runnable.find((s) => s.name === arg);
if (!server || !server.dev) {
  console.error(c.red(`unknown dev server "${arg}". Known: ${runnable.map((s) => s.name).join(", ")}`));
  process.exit(1);
}

console.log(c.cyan(`› dev ${server.name}: ${server.dev}`));
const res = spawnSync(server.dev, { cwd: server.dir, shell: true, stdio: "inherit" });
process.exit(res.status ?? 1);
