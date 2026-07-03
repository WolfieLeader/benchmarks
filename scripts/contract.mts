// Contract conformance harness (Phase 0A).
//
// Given an entry name (or `all`), this script:
//   1. resolves entry -> image/port from config/config.json,
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
import { connect } from "node:net";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

type ServerEntry = { name: string; image: string; port: number };
type Config = { servers: ServerEntry[]; databases: string[] };
type EntryResult = { name: string; passed: number; failed: number; ok: boolean; note: string };

const scriptDir = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(scriptDir, "..");
const benchmarksDir = join(repoRoot, "benchmarks");
const contractDir = join(repoRoot, "contract");
const testFilesDir = join(repoRoot, "test-files");

// Published host ports for the running databases (infra/compose/databases.yml).
const DB_HOST_PORTS: Record<string, number> = {
  postgres: 5432,
  mongodb: 27017,
  redis: 6379,
  cassandra: 9042
};

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

// image -> server source directory (Docker build context), derivable from the
// image name and the http-servers/<lang>/<dir> layout.
function serverDir(image: string): string {
  if (image.startsWith("go-")) return join(repoRoot, "http-servers", "go", image.slice(3));
  if (image.startsWith("python-")) return join(repoRoot, "http-servers", "python", image.slice(7));
  return join(repoRoot, "http-servers", "typescript", image);
}

function tcpReachable(port: number, timeoutMs: number): Promise<boolean> {
  return new Promise((resolve) => {
    const socket = connect({ host: "127.0.0.1", port });
    const done = (ok: boolean) => {
      socket.destroy();
      resolve(ok);
    };
    socket.setTimeout(timeoutMs);
    socket.once("connect", () => done(true));
    socket.once("timeout", () => done(false));
    socket.once("error", () => done(false));
  });
}

async function dbPreflight(databases: string[]): Promise<void> {
  const unreachable: string[] = [];
  for (const db of databases) {
    const port = DB_HOST_PORTS[db];
    if (port === undefined) fail(`unknown database "${db}" in config — no host port mapping`);
    if (!(await tcpReachable(port, 2000))) unreachable.push(`${db} (:${port})`);
  }
  if (unreachable.length > 0) {
    fail(`databases not reachable: ${unreachable.join(", ")}\n  Start them with:  just db-up`);
  }
}

// The server container reaches the DBs by compose service name (postgres,
// mongodb, ...), so it must join the network the DB containers live on.
// Detect it from the running postgres service container instead of hardcoding
// the compose project name (which differs between `just db-up` and the client).
function detectDbNetwork(): string {
  const ps = spawnSync(
    "docker",
    ["ps", "--filter", "label=com.docker.compose.service=postgres", "--format", "{{.Names}}"],
    { encoding: "utf8" }
  );
  const name = ps.stdout.trim().split("\n")[0]?.trim();
  if (!name) fail("could not find the running postgres container to detect its network — is `just db-up` up?");
  const inspect = spawnSync(
    "docker",
    ["inspect", name, "--format", "{{range $k,$v := .NetworkSettings.Networks}}{{$k}}\n{{end}}"],
    { encoding: "utf8" }
  );
  const network = inspect.stdout.trim().split("\n")[0]?.trim();
  if (!network) fail(`could not detect the docker network for container ${name}`);
  return network;
}

function imageExists(image: string): boolean {
  return spawnSync("docker", ["image", "inspect", image], { stdio: "ignore" }).status === 0;
}

function buildImage(image: string): void {
  const dir = serverDir(image);
  console.log(`\x1b[36m›\x1b[0m building image ${image} from ${dir} ...`);
  const res = spawnSync("docker", ["build", "-t", image, dir], { stdio: "inherit" });
  if (res.status !== 0) throw new HarnessError("build", `docker build failed for ${image}`);
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
  const res = spawnSync("go", ["build", "-o", binPath, "./cmd/main.go"], {
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
    const relay = (chunk: Buffer, out: NodeJS.WriteStream) => {
      const s = chunk.toString();
      buf += s;
      out.write(s);
    };
    child.stdout.on("data", (c) => relay(c, process.stdout));
    child.stderr.on("data", (c) => relay(c, process.stderr));
    child.on("close", (code) => {
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

async function runEntry(
  entry: ServerEntry,
  network: string,
  binPath: string,
  databases: string[]
): Promise<EntryResult> {
  console.log(`\n\x1b[1m━━ ${entry.name} (${entry.image}, port ${entry.port}) ━━\x1b[0m`);
  if (!imageExists(entry.image) || forceBuild) buildImage(entry.image);

  const id = startContainer(entry.image, entry.port, network);
  activeContainer = id;
  try {
    console.log(`\x1b[36m›\x1b[0m container ${id.slice(0, 12)} up; waiting for health ...`);
    await waitHealthy(entry.port, databases);
    const { code, passed, failed } = await runConformance(binPath, entry.port);
    return { name: entry.name, passed, failed, ok: code === 0, note: code === 0 ? "" : `${failed} failed` };
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

  await dbPreflight(databases);
  const network = detectDbNetwork();
  console.log(`\x1b[36m›\x1b[0m databases reachable; server network = ${network}`);

  let entries: ServerEntry[];
  if (target === "all") {
    entries = config.servers;
  } else {
    const match = config.servers.find((s) => s.name === target || s.image === target);
    if (!match) {
      fail(`unknown entry "${target}". Known: ${config.servers.map((s) => s.name).join(", ")}, all`);
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

main().catch((err) => {
  console.error(err);
  teardownAndExit(1);
});
