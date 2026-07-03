// Contract conformance harness (Phase 0A).
//
// Given an entry name (or `all`), this script:
//   1. resolves entry -> image/port from the discovered bench.json roster
//      (scripts/lib.mts, PLAN §11.1); config.json still supplies the DB list,
//   2. builds the server image if missing (or when --build is passed),
//   3. starts the container on the running databases network with the same
//      baked env the benchmark uses (DB host = compose service name),
//   4. waits for /health (server + each database) with a real timeout,
//   5. runs the Go client's conformance command against the mapped host port,
//   6. always tears the container down (success, failure, or Ctrl-C).
//
// Run with Node 26 native type-stripping: `node scripts/contract.mts <entry|all>`.
// No tsx, no build step, no npm deps — node builtins + docker/go via child_process.

import { spawn, spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { repoRoot, SERVERS, type Server } from "./lib.mts";

// A fully-resolved server entry: the roster rows that carry both an image and a
// port (i.e. the discovered servers, not the benchmark/root helper targets).
type Entry = { name: string; image: string; port: number; dir: string };
type Config = { databases: string[] };
type EntryResult = { name: string; passed: number; failed: number; ok: boolean; note: string };

const benchmarksDir = join(repoRoot, "benchmarks");
const contractDir = join(repoRoot, "contract");
const testFilesDir = join(repoRoot, "test-files");

// The compose file our databases stack is defined in — used to pick OUR stack
// when other compose projects on this host also run a postgres service.
const dbComposeFile = join(repoRoot, "infra", "compose", "databases.yml");

const HEALTH_TIMEOUT_MS = 45_000;
const HEALTH_INTERVAL_MS = 300;

function fail(message: string): never {
  console.error(`\x1b[31m✗\x1b[0m ${message}`);
  process.exit(1);
}

function loadConfig(): Config {
  const raw = readFileSync(join(repoRoot, "config", "config.json"), "utf8");
  return JSON.parse(raw) as Config;
}

// The conformance roster: discovered servers (bench.json) that carry an image and
// a port. The benchmark/root helper rows in SERVERS have neither and drop out.
function roster(): Entry[] {
  return SERVERS.filter((s): s is Server & { image: string; port: number } => !!s.image && s.port !== undefined).map(
    (s) => ({ name: s.name, image: s.image, port: s.port, dir: s.dir })
  );
}

// Case-insensitive path handling: macOS filesystems are case-insensitive and
// compose records whatever casing it was invoked from (e.g. ~/dev vs ~/Dev).
// The stack may have been created from ANY checkout of this repo (main clone or
// a .claude/worktrees agent worktree), so "ours" means: the recorded compose
// file is our databases.yml inside the main repo root (worktrees live under it).
const mainRepoRoot = (() => {
  const res = spawnSync("git", ["rev-parse", "--path-format=absolute", "--git-common-dir"], {
    cwd: repoRoot,
    encoding: "utf8"
  });
  return res.status === 0 ? dirname(res.stdout.trim()) : repoRoot;
})();

function isOurComposeFile(path: string): boolean {
  const p = resolve(path).toLowerCase();
  const suffix = join("infra", "compose", "databases.yml").toLowerCase();
  return p.startsWith(resolve(mainRepoRoot).toLowerCase()) && p.endsWith(suffix);
}

type DbStack = { project: string; network: string };

// The server container reaches the DBs by compose service name (postgres,
// mongodb, ...), so it must join the network OUR DB containers live on.
// Other compose projects on this host may also run a postgres service (other
// repos, the Phase 2 metrics-postgres), so "first postgres container" is not
// safe: match on the com.docker.compose.project.config_files label pointing at
// our infra/compose/databases.yml, and fail loud on zero or ambiguous matches.
function detectDbStack(): DbStack {
  const ps = spawnSync(
    "docker",
    ["ps", "--filter", "label=com.docker.compose.service=postgres", "--format", "{{.ID}}"],
    { encoding: "utf8" }
  );
  const ids = ps.stdout.trim().split("\n").filter(Boolean);
  if (ids.length === 0) {
    fail("no running postgres container found — start the databases with:  just db-up");
  }

  type Candidate = { name: string; project: string; configFiles: string; network: string };
  const candidates: Candidate[] = [];
  for (const id of ids) {
    const inspect = spawnSync(
      "docker",
      [
        "inspect",
        id,
        "--format",
        `{{.Name}}\t{{index .Config.Labels "com.docker.compose.project"}}\t{{index .Config.Labels "com.docker.compose.project.config_files"}}\t{{range $k,$v := .NetworkSettings.Networks}}{{$k}}{{end}}`
      ],
      { encoding: "utf8" }
    );
    const [name = "", project = "", configFiles = "", network = ""] = inspect.stdout.trim().split("\t");
    candidates.push({ name: name.replace(/^\//, ""), project, configFiles, network });
  }

  // config_files is a comma-separated list; ours is a single file.
  const ours = candidates.filter((c) => c.configFiles.split(",").some((f) => isOurComposeFile(f.trim())));

  if (ours.length === 0) {
    const list = candidates.map((c) => `    ${c.name} (project=${c.project}, config=${c.configFiles})`).join("\n");
    fail(
      `found running postgres container(s), but none belong to this repo's databases stack (${dbComposeFile}):\n${list}\n  Start ours with:  just db-up`
    );
  }
  if (ours.length > 1) {
    const list = ours.map((c) => `    ${c.name} (project=${c.project}, network=${c.network})`).join("\n");
    fail(`multiple postgres containers match ${dbComposeFile} — ambiguous stack, stop the extras:\n${list}`);
  }
  const stack = ours[0];
  if (!stack.network) fail(`could not detect the docker network for container ${stack.name}`);
  return { project: stack.project, network: stack.network };
}

// Preflight the SAME stack whose network the server will join: every database
// service must have a running, healthy container in that compose project.
function dbPreflight(stack: DbStack, databases: string[]): void {
  const bad: string[] = [];
  for (const db of databases) {
    const ps = spawnSync(
      "docker",
      [
        "ps",
        "--filter",
        `label=com.docker.compose.project=${stack.project}`,
        "--filter",
        `label=com.docker.compose.service=${db}`,
        "--format",
        "{{.ID}}"
      ],
      { encoding: "utf8" }
    );
    const id = ps.stdout.trim().split("\n")[0]?.trim();
    if (!id) {
      bad.push(`${db} (no running container in project ${stack.project})`);
      continue;
    }
    const health = spawnSync("docker", ["inspect", id, "--format", "{{.State.Health.Status}}"], {
      encoding: "utf8"
    }).stdout.trim();
    if (health !== "healthy") bad.push(`${db} (health: ${health || "unknown"})`);
  }
  if (bad.length > 0) {
    fail(`databases not ready: ${bad.join(", ")}\n  Start them with:  just db-up`);
  }
}

function imageExists(image: string): boolean {
  return spawnSync("docker", ["image", "inspect", image], { stdio: "ignore" }).status === 0;
}

function buildImage(entry: Entry): void {
  console.log(`\x1b[36m›\x1b[0m building image ${entry.image} from ${entry.dir} ...`);
  const res = spawnSync("docker", ["build", "-t", entry.image, entry.dir], { stdio: "inherit" });
  if (res.status !== 0) throw new HarnessError("build", `docker build failed for ${entry.image}`);
}

function startContainer(image: string, port: number, network: string): string {
  // -d --rm so `docker stop` auto-removes; map host port = container port
  // (the image's baked PORT env), join the DB network for service-name DNS.
  const res = spawnSync("docker", ["run", "-d", "--rm", "-p", `${port}:${port}`, "--network", network, image], {
    encoding: "utf8"
  });
  if (res.status !== 0) {
    throw new HarnessError("start", `docker run failed for ${image}:\n${res.stderr.trim()}`);
  }
  return res.stdout.trim();
}

function stopContainer(id: string): void {
  if (!id) return;
  spawnSync("docker", ["stop", "-t", "2", id], { stdio: "ignore" });
}

async function httpOk(url: string): Promise<boolean> {
  try {
    const res = await fetch(url, { signal: AbortSignal.timeout(2000) });
    await res.text();
    return res.status === 200;
  } catch {
    return false;
  }
}

async function waitHealthy(port: number, databases: string[]): Promise<void> {
  const base = `http://localhost:${port}`;
  const deadline = Date.now() + HEALTH_TIMEOUT_MS;
  let lastErr = "server /health never returned 200";
  while (Date.now() < deadline) {
    if (await httpOk(`${base}/health`)) {
      let allDbs = true;
      for (const db of databases) {
        if (!(await httpOk(`${base}/db/${db}/health`))) {
          allDbs = false;
          lastErr = `database ${db} health check (/db/${db}/health) never returned 200`;
          break;
        }
      }
      if (allDbs) return;
    }
    await new Promise((r) => setTimeout(r, HEALTH_INTERVAL_MS));
  }
  throw new HarnessError("health", `never became healthy within ${HEALTH_TIMEOUT_MS / 1000}s: ${lastErr}`);
}

// Build the conformance binary once and reuse it across every entry (avoids a
// `go run` recompile per server). Returns the binary path.
function buildConformanceBinary(): string {
  const binPath = join(benchmarksDir, "bin", "conformance");
  console.log("\x1b[36m›\x1b[0m building Go conformance binary ...");
  const res = spawnSync("go", ["build", "-o", binPath, "./cmd"], {
    cwd: benchmarksDir,
    stdio: "inherit"
  });
  if (res.status !== 0) fail("failed to build the Go conformance binary");
  return binPath;
}

// Run the conformance binary, streaming its output while capturing it to parse
// the "N passed, M failed" summary line for the final table.
function runConformance(binPath: string, port: number): Promise<{ code: number; passed: number; failed: number }> {
  return new Promise((resolve) => {
    const child = spawn(
      binPath,
      [
        "--conformance",
        `--base-url=http://localhost:${port}`,
        `--contract-dir=${contractDir}`,
        `--test-files-dir=${testFilesDir}`
      ],
      { cwd: benchmarksDir }
    );
    let buf = "";
    let settled = false;
    const relay = (chunk: Buffer, out: NodeJS.WriteStream) => {
      const s = chunk.toString();
      buf += s;
      out.write(s);
    };
    child.stdout.on("data", (c) => relay(c, process.stdout));
    child.stderr.on("data", (c) => relay(c, process.stderr));
    // Spawn failure (ENOENT/EACCES) emits 'error' instead of 'close'; resolve
    // as a failure so the caller's finally still tears the container down.
    child.on("error", (err) => {
      if (settled) return;
      settled = true;
      console.error(`\x1b[31m✗ failed to run conformance binary: ${err.message}\x1b[0m`);
      resolve({ code: 1, passed: 0, failed: 0 });
    });
    child.on("close", (code) => {
      if (settled) return;
      settled = true;
      const m = buf.match(/(\d+)\s+passed,\s+(\d+)\s+failed/);
      const passed = m ? Number(m[1]) : 0;
      const failed = m ? Number(m[2]) : 0;
      resolve({ code: code ?? 1, passed, failed });
    });
  });
}

class HarnessError extends Error {
  phase: string;
  constructor(phase: string, message: string) {
    super(message);
    this.phase = phase;
  }
}

// Tracks the currently-running container so signal handlers can tear it down.
let activeContainer = "";

async function runEntry(entry: Entry, network: string, binPath: string, databases: string[]): Promise<EntryResult> {
  console.log(`\n\x1b[1m━━ ${entry.name} (${entry.image}, port ${entry.port}) ━━\x1b[0m`);
  if (!imageExists(entry.image) || forceBuild) buildImage(entry);

  const id = startContainer(entry.image, entry.port, network);
  activeContainer = id;
  try {
    console.log(`\x1b[36m›\x1b[0m container ${id.slice(0, 12)} up; waiting for health ...`);
    await waitHealthy(entry.port, databases);
    const { code, passed, failed } = await runConformance(binPath, entry.port);
    const note = code === 0 ? "" : failed > 0 ? `${failed} failed` : "conformance run error";
    return { name: entry.name, passed, failed, ok: code === 0, note };
  } finally {
    stopContainer(id);
    activeContainer = "";
  }
}

function printTable(results: EntryResult[]): void {
  console.log(`\n\x1b[1m━━ Contract results ━━\x1b[0m`);
  const nameW = Math.max(6, ...results.map((r) => r.name.length));
  for (const r of results) {
    const status = r.ok ? "\x1b[32mPASS\x1b[0m" : "\x1b[31mFAIL\x1b[0m";
    const counts = `${r.passed}/${r.passed + r.failed}`;
    console.log(`  ${r.name.padEnd(nameW)}  ${status}  ${counts.padStart(7)}  ${r.note}`);
  }
}

let forceBuild = false;

async function main(): Promise<void> {
  const args = process.argv.slice(2).filter((a) => {
    if (a === "--build") {
      forceBuild = true;
      return false;
    }
    return true;
  });
  const target = args[0] ?? "all";

  const config = loadConfig();
  const databases = config.databases;

  const stack = detectDbStack();
  dbPreflight(stack, databases);
  const network = stack.network;
  console.log(`\x1b[36m›\x1b[0m databases healthy (project ${stack.project}); server network = ${network}`);

  const allEntries = roster();
  let entries: Entry[];
  if (target === "all") {
    entries = allEntries;
  } else {
    const match = allEntries.find((s) => s.name === target || s.image === target);
    if (!match) {
      fail(`unknown entry "${target}". Known: ${allEntries.map((s) => s.name).join(", ")}, all`);
    }
    entries = [match];
  }

  const binPath = buildConformanceBinary();

  const results: EntryResult[] = [];
  for (const entry of entries) {
    try {
      results.push(await runEntry(entry, network, binPath, databases));
    } catch (err) {
      const phase = err instanceof HarnessError ? err.phase : "error";
      const msg = err instanceof Error ? err.message : String(err);
      console.error(`\x1b[31m✗ ${entry.name} [${phase}] ${msg}\x1b[0m`);
      results.push({ name: entry.name, passed: 0, failed: 0, ok: false, note: `${phase}: ${msg.split("\n")[0]}` });
    }
  }

  printTable(results);
  const anyFailed = results.some((r) => !r.ok);
  process.exit(anyFailed ? 1 : 0);
}

// Always tear down the active container on interrupt / fatal error.
function teardownAndExit(code: number): never {
  if (activeContainer) {
    console.error(`\n\x1b[33m› tearing down container ${activeContainer.slice(0, 12)} ...\x1b[0m`);
    stopContainer(activeContainer);
    activeContainer = "";
  }
  process.exit(code);
}
process.on("SIGINT", () => teardownAndExit(130));
process.on("SIGTERM", () => teardownAndExit(143));
// Safety net: no synchronous throw in an event callback may ever leak a container.
process.on("uncaughtException", (err) => {
  console.error(err);
  teardownAndExit(1);
});
process.on("unhandledRejection", (reason) => {
  console.error(reason);
  teardownAndExit(1);
});

main().catch((err) => {
  console.error(err);
  teardownAndExit(1);
});
