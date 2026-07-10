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
import { join } from "node:path";
import { dbPreflight, detectDbStack, repoRoot, SERVERS, type Server } from "./lib.mts";

// A fully-resolved server entry: the roster rows that carry both an image and a
// port (i.e. the discovered servers, not the benchmark/root helper targets).
type Entry = { name: string; image: string; port: number; dir: string; web: boolean };
type Config = { databases: string[] };
type EntryResult = { name: string; passed: number; failed: number; ok: boolean; note: string };

const benchmarkDir = join(repoRoot, "benchmark");
const contractDir = join(repoRoot, "contract");
const testFilesDir = join(repoRoot, "contract", "test-files");

const HEALTH_TIMEOUT_MS = 45_000;
const HEALTH_INTERVAL_MS = 300;

// Shared HS256 secret the web suite signs and verifies with. Passed to BOTH the
// container (JWT_SECRET env) and the conformance runner (--jwt-secret) so the
// server and the $jwt matcher agree on the key. Mirrors conformance.DefaultJWTSecret
// and the shared env modules' JWT_SECRET dev default (shared/{typescript,python,rust,go}).
const JWT_SECRET = "benchmarks-shared-jwt-secret-dev-default";

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
    (s) => ({ name: s.name, image: s.image, port: s.port, dir: s.dir, web: s.web ?? false })
  );
}

// DB-stack detection + preflight (detectDbStack/dbPreflight) live in lib.mts,
// shared with calibrate.mts — same "fail loud on zero or ambiguous stacks" rules.

function imageExists(image: string): boolean {
  return spawnSync("docker", ["image", "inspect", image], { stdio: "ignore" }).status === 0;
}

function buildImage(entry: Entry): void {
  console.log(`\x1b[36m›\x1b[0m building image ${entry.image} from ${entry.dir} ...`);
  // Root-context build (PLAN §2.4): context = repo root, Dockerfile selected with
  // -f, so the image can COPY its own folder and (from 0D) the shared/<lang> layer.
  const res = spawnSync("docker", ["build", "-t", entry.image, "-f", join(entry.dir, "Dockerfile"), repoRoot], {
    stdio: "inherit"
  });
  if (res.status !== 0) throw new HarnessError("build", `docker build failed for ${entry.image}`);
}

function startContainer(image: string, port: number, network: string): string {
  // -d --rm so `docker stop` auto-removes; map host port = container port
  // (the image's baked PORT env), join the DB network for service-name DNS.
  // JWT_SECRET is part of the env contract (web suite): harmless for servers
  // that don't read it, and it must match the runner's --jwt-secret for web servers.
  const res = spawnSync(
    "docker",
    ["run", "-d", "--rm", "-p", `${port}:${port}`, "-e", `JWT_SECRET=${JWT_SECRET}`, "--network", network, image],
    { encoding: "utf8" }
  );
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
  const binPath = join(benchmarkDir, "bin", "conformance");
  console.log("\x1b[36m›\x1b[0m building Go conformance binary ...");
  const res = spawnSync("go", ["build", "-o", binPath, "./cmd"], {
    cwd: benchmarkDir,
    stdio: "inherit"
  });
  if (res.status !== 0) fail("failed to build the Go conformance binary");
  return binPath;
}

// Run the conformance binary, streaming its output while capturing it to parse
// the "N passed, M failed" summary line for the final table. The web suite is
// gated per server: unless the manifest declares web support, it is skipped
// (--skip-suite=web) so servers that don't implement /html,/jwt/*,/validate,
// /compute stay green. --jwt-secret must match the container's JWT_SECRET.
function runConformance(
  binPath: string,
  port: number,
  web: boolean
): Promise<{ code: number; passed: number; failed: number }> {
  return new Promise((resolve) => {
    const args = [
      "--conformance",
      `--base-url=http://localhost:${port}`,
      `--contract-dir=${contractDir}`,
      `--test-files-dir=${testFilesDir}`,
      `--jwt-secret=${JWT_SECRET}`
    ];
    if (!web) args.push("--skip-suite=web");
    const child = spawn(binPath, args, { cwd: benchmarkDir });
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
    const { code, passed, failed } = await runConformance(binPath, entry.port, entry.web);
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
