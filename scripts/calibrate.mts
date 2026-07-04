// Calibration gate (PLAN §7.1 / §7.6): cross-validate the custom Go load
// generator against oha, the only maintained reference that measures the same
// thing we do (constant-arrival-rate + latency from the INTENDED send time,
// wrk2 semantics via --latency-correction).
//
// Given a server entry (default go-chi), this script:
//   1. starts ONE server container with the calibration limits (both
//      generators hit the identical instance via the client's --target mode),
//   2. Experiment A — agreement: client v2 and oha run the same closed shape
//      (concurrency C) and the same open shape (rate R, CO-corrected); open
//      mode is the pass/fail gate, closed mode is reported (the client's
//      response-validation tax makes closed throughput legitimately lower),
//   3. Experiment B — ceiling: steps the client's open rate until either wall
//      signal fires — dropped iterations (queue wall) or offered_rate
//      decoupling from target (dispatcher wall) — then fires oha at the same
//      rate to attribute the wall (client vs server),
//   4. prints the comparison table and exits non-zero on open-mode drift.
//
// The measured numbers and tolerance rationale live in docs/calibration.md —
// re-run this whenever benchmark/internal/client's hot path changes.
//
// Run with Node 26 native type-stripping: `node scripts/calibrate.mts [entry]`.
// No npm deps — node builtins + docker/go/oha via child_process.

import { spawnSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { c, dbPreflight, detectDbStack, repoRoot, SERVERS, type Server } from "./lib.mts";

// ── knobs (documented in docs/calibration.md) ──
const OPEN_RATE = 5000; // Experiment A open-mode rate — far below any ceiling
const RPS_TOLERANCE = 0.05; // open-mode RPS must agree within ±5%
const LATENCY_TOLERANCE = 0.1; // open-mode p50/p99 within ±10%…
const LATENCY_FLOOR_NS = 1_000_000; // …or within 1ms absolute (timer granularity floor, wrk2 README)
const MAX_IN_FLIGHT = 512; // written into every open config AND passed to oha -c — one value, same shape
const CEILING_RATES = [5_000, 10_000, 20_000, 40_000, 80_000, 160_000];
// Two distinct wall signals, either ends the search:
// - dropped iterations: the QUEUE wall — workers can't absorb on-time arrivals
//   (the arrival clock itself never blocks, k6 semantics, so drops are how
//   server-side saturation shows up);
// - offered_rate decoupling from target: the DISPATCHER wall — the client's
//   own arrival clock produced arrivals late, so the queue never even filled.
const DROP_RATIO_OK = 0.001; // <=0.1% dropped = queue kept up
const OFFERED_TRACK_OK = 0.99; // offered/target >= 99% = dispatcher kept schedule
const OFFERED_RATIO_OK = 0.95; // oha attribution: achieved/target below this = it hit the wall too
const HEALTH_TIMEOUT_MS = 45_000;

const quick = process.argv.includes("--quick"); // smoke-test the harness, NOT a valid calibration
const MEASURE_DURATION = quick ? "5s" : "30s";
const CEILING_DURATION = quick ? "3s" : "10s";

// fail() throws (instead of process.exit) so the container-cleanup `finally`
// in main() runs on every failure path — a leaked server container holds the
// host port and breaks the next calibrate/contract run.
class CalibrationError extends Error {}

function fail(message: string): never {
  throw new CalibrationError(message);
}

function info(message: string): void {
  console.log(`${c.cyan("›")} ${message}`);
}

// ── result shapes (ours: benchmark/internal/summary; theirs: oha --output-format json) ──

type OurStats = {
  count: number;
  total_count: number;
  rps: number;
  p50_ns?: number;
  p99_ns?: number;
  success_rate: number;
};
type OurEndpoint = {
  name: string;
  stats?: OurStats;
  open?: {
    target_rate: number;
    offered_rate: number;
    attempted: number;
    dropped_iterations: number;
    response?: OurStats;
  };
  failure_count?: number;
  last_error?: string;
};
type OurResult = { name: string; error?: string; results?: OurEndpoint[] };

type OhaResult = {
  summary: { successRate: number; total: number; requestsPerSec: number };
  latencyPercentiles: Record<string, number>; // seconds
  statusCodeDistribution: Record<string, number>;
};

type CalibrationConfig = {
  benchmark: {
    concurrency: number;
    duration_per_endpoint: string;
    request_timeout: string;
    load?: { mode: string; rate?: number; max_in_flight?: number };
    [key: string]: unknown;
  };
  container: { cpu_limit: number; memory_limit: string };
  endpoints: Record<string, { route: string }>;
  [key: string]: unknown;
};

// ── setup helpers ──

function entryFor(target: string): Server & { image: string; port: number } {
  const match = SERVERS.find(
    (s): s is Server & { image: string; port: number } =>
      s.name === target && !!s.image && s.port !== undefined
  );
  if (!match) {
    const names = SERVERS.filter((s) => s.image).map((s) => s.name);
    fail(`unknown server "${target}". Known: ${names.join(", ")}`);
  }
  return match;
}

function ensureOha(): string {
  const res = spawnSync("oha", ["--version"], { encoding: "utf8" });
  if (res.status !== 0) fail("oha not found — install it with:  brew install oha");
  return res.stdout.trim();
}

function ensureImage(entry: Server & { image: string }): void {
  if (spawnSync("docker", ["image", "inspect", entry.image], { stdio: "ignore" }).status === 0) return;
  info(`building image ${entry.image} ...`);
  const res = spawnSync("docker", ["build", "-t", entry.image, "-f", join(entry.dir, "Dockerfile"), repoRoot], {
    stdio: "inherit"
  });
  if (res.status !== 0) fail(`docker build failed for ${entry.image}`);
}

function buildClient(): string {
  const binPath = join(repoRoot, "benchmark", "bin", "benchmark");
  info("building Go benchmark client ...");
  const res = spawnSync("go", ["build", "-o", binPath, "./cmd"], {
    cwd: join(repoRoot, "benchmark"),
    encoding: "utf8"
  });
  if (res.status !== 0) fail(`go build failed:\n${res.stderr}`);
  return binPath;
}

function startContainer(image: string, port: number, network: string, cpu: number, memory: string): string {
  const res = spawnSync(
    "docker",
    ["run", "-d", "--rm", "-p", `${port}:${port}`, "--network", network, "--cpus", String(cpu), "--memory", memory, image],
    { encoding: "utf8" }
  );
  if (res.status !== 0) fail(`docker run failed for ${image}:\n${res.stderr.trim()}`);
  return res.stdout.trim();
}

function stopContainer(id: string): void {
  if (!id) return;
  spawnSync("docker", ["stop", "-t", "2", id], { stdio: "ignore" });
}

async function waitHealthy(base: string): Promise<void> {
  const deadline = Date.now() + HEALTH_TIMEOUT_MS;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(`${base}/health`, { signal: AbortSignal.timeout(2000) });
      await res.text();
      if (res.status === 200) return;
    } catch {
      // not up yet
    }
    await new Promise((r) => setTimeout(r, 300));
  }
  fail(`server /health never returned 200 within ${HEALTH_TIMEOUT_MS / 1000}s`);
}

// ── run legs ──

function runClient(bin: string, base: string, configPath: string, outDir: string): OurEndpoint {
  const res = spawnSync(bin, [`--target=${base}`, `--config=${configPath}`, `--results-dir=${outDir}`], {
    cwd: join(repoRoot, "benchmark"),
    stdio: ["ignore", "inherit", "inherit"]
  });
  if (res.status !== 0) fail(`benchmark client exited ${res.status} (config ${configPath})`);
  const parsed = JSON.parse(readFileSync(join(outDir, "target.json"), "utf8")) as OurResult;
  if (parsed.error) fail(`client run errored: ${parsed.error}`);
  const ep = parsed.results?.[0];
  if (!ep?.stats) fail(`client result has no endpoint stats (${join(outDir, "target.json")})`);
  return ep;
}

function runOha(url: string, duration: string, args: string[]): OhaResult {
  const full = ["-z", duration, "--no-tui", "--output-format", "json", ...args, url];
  info(`oha ${full.join(" ")}`);
  const res = spawnSync("oha", full, { encoding: "utf8", maxBuffer: 64 * 1024 * 1024 });
  if (res.status !== 0) fail(`oha exited ${res.status}:\n${res.stderr}`);
  const parsed = JSON.parse(res.stdout) as OhaResult;
  if (!parsed.summary || !parsed.latencyPercentiles) fail("oha JSON missing summary/latencyPercentiles");
  return parsed;
}

// Layer-1 sanity: both tools must have measured 100%-successful work, or the
// timing comparison below is comparing different experiments.
function sanity(label: string, ours: OurEndpoint, oha: OhaResult, expectStatus: string): void {
  const bad: string[] = [];
  if (ours.stats && ours.stats.success_rate < 1) {
    bad.push(`client success_rate ${ours.stats.success_rate} (${ours.failure_count} failures, last: ${ours.last_error ?? "?"})`);
  }
  if (oha.summary.successRate < 1) bad.push(`oha successRate ${oha.summary.successRate}`);
  const badCodes = Object.keys(oha.statusCodeDistribution).filter((k) => k !== expectStatus);
  if (badCodes.length > 0) bad.push(`oha saw non-${expectStatus} statuses: ${badCodes.join(", ")}`);
  if (bad.length > 0) fail(`${label}: sanity failed — ${bad.join("; ")}`);
}

// ── comparison ──

type Row = { leg: string; metric: string; ours: number; theirs: number; unit: "rps" | "ns"; gate: boolean };

function delta(row: Row): number {
  return row.theirs === 0 ? Number.POSITIVE_INFINITY : (row.ours - row.theirs) / row.theirs;
}

function withinTolerance(row: Row): boolean {
  const d = Math.abs(delta(row));
  if (row.unit === "rps") return d <= RPS_TOLERANCE;
  return d <= LATENCY_TOLERANCE || Math.abs(row.ours - row.theirs) <= LATENCY_FLOOR_NS;
}

function fmt(v: number, unit: "rps" | "ns"): string {
  if (unit === "rps") return v.toFixed(0);
  return `${(v / 1e6).toFixed(2)}ms`;
}

function printRows(rows: Row[]): boolean {
  let ok = true;
  console.log(`\n${c.bold("━━ Calibration: client v2 vs oha ━━")}`);
  console.log(
    `  ${"leg".padEnd(8)}${"metric".padEnd(8)}${"client v2".padStart(12)}${"oha".padStart(12)}${"Δ".padStart(9)}  verdict`
  );
  for (const row of rows) {
    const pass = withinTolerance(row);
    const d = `${(delta(row) * 100).toFixed(1)}%`;
    let verdict: string;
    if (row.gate) {
      verdict = pass ? c.green("PASS") : c.red("FAIL");
      if (!pass) ok = false;
    } else {
      verdict = c.dim(pass ? "info (agrees)" : "info (expected gap)");
    }
    console.log(
      `  ${row.leg.padEnd(8)}${row.metric.padEnd(8)}${fmt(row.ours, row.unit).padStart(12)}${fmt(row.theirs, row.unit).padStart(12)}${d.padStart(9)}  ${verdict}`
    );
  }
  return ok;
}

// ── main ──

async function main(): Promise<void> {
  const target = process.argv.slice(2).find((a) => !a.startsWith("-")) ?? "go-chi";
  const entry = entryFor(target);

  const ohaVersion = ensureOha();
  info(`reference: ${ohaVersion}`);
  if (quick) console.log(c.yellow("  --quick: short runs — harness smoke test only, NOT a valid calibration"));

  const baseConfig = JSON.parse(readFileSync(join(repoRoot, "config", "calibration.json"), "utf8")) as CalibrationConfig;
  baseConfig.benchmark.duration_per_endpoint = MEASURE_DURATION;
  const concurrency = baseConfig.benchmark.concurrency;
  const route = Object.values(baseConfig.endpoints)[0].route.split(" ")[1];

  // The server needs the DB stack to boot (its pools connect at startup) even
  // though the calibration load only touches the DB-free health endpoint.
  const fullConfig = JSON.parse(readFileSync(join(repoRoot, "config", "config.json"), "utf8")) as {
    databases: string[];
  };
  const stack = detectDbStack();
  dbPreflight(stack, fullConfig.databases);

  ensureImage(entry);
  const bin = buildClient();

  const runDir = join(repoRoot, "results", `calibration-${new Date().toISOString().replace(/[:.]/g, "-")}`);
  mkdirSync(runDir, { recursive: true });
  info(`run artifacts: ${runDir}`);

  const containerId = startContainer(
    entry.image,
    entry.port,
    stack.network,
    baseConfig.container.cpu_limit,
    baseConfig.container.memory_limit
  );
  const cleanup = (): void => stopContainer(containerId);
  process.on("SIGINT", () => {
    cleanup();
    process.exit(130);
  });

  try {
    const base = `http://localhost:${entry.port}`;
    const url = `${base}${route}`;
    await waitHealthy(base);
    info(`server ${entry.name} up (container ${containerId.slice(0, 12)}); target ${url}`);

    // ── Experiment A: agreement ──
    console.log(`\n${c.bold(`━━ Experiment A: agreement (closed c=${concurrency}, open rate=${OPEN_RATE}) ━━`)}`);

    const closedConfigPath = join(runDir, "closed.json");
    writeFileSync(closedConfigPath, JSON.stringify(baseConfig, null, 2));
    const closedOurs = runClient(bin, base, closedConfigPath, join(runDir, "closed"));
    const closedOha = runOha(url, MEASURE_DURATION, ["-c", String(concurrency)]);
    sanity("closed", closedOurs, closedOha, "200");

    const openConfig = structuredClone(baseConfig);
    openConfig.benchmark.load = { mode: "open", rate: OPEN_RATE, max_in_flight: MAX_IN_FLIGHT };
    const openConfigPath = join(runDir, "open.json");
    writeFileSync(openConfigPath, JSON.stringify(openConfig, null, 2));
    const openOurs = runClient(bin, base, openConfigPath, join(runDir, "open"));
    // oha's CO-corrected mode mirrors ours: -q total rate + latency from the
    // scheduled send time; -c matches our max_in_flight worker bound.
    const openOha = runOha(url, MEASURE_DURATION, [
      "-q",
      String(OPEN_RATE),
      "-c",
      String(MAX_IN_FLIGHT),
      "--latency-correction"
    ]);
    sanity("open", openOurs, openOha, "200");
    if (!openOurs.open) fail("client open-mode run produced no open stats");
    if (openOurs.open.dropped_iterations > 0) {
      fail(`client dropped ${openOurs.open.dropped_iterations} iterations at rate ${OPEN_RATE} — not a calibration rate`);
    }

    // Open mode gates: same load shape, same CO semantics — the numbers must
    // agree. Closed mode is informational: the client's response-validation
    // work makes its closed loop legitimately slower (docs/calibration.md).
    const s = (e: OurEndpoint): OurStats => e.stats as OurStats;
    const resp = openOurs.open.response ?? s(openOurs);
    const rows: Row[] = [
      { leg: "open", metric: "rps", ours: s(openOurs).rps, theirs: openOha.summary.requestsPerSec, unit: "rps", gate: true },
      { leg: "open", metric: "p50", ours: resp.p50_ns ?? 0, theirs: openOha.latencyPercentiles.p50 * 1e9, unit: "ns", gate: true },
      { leg: "open", metric: "p99", ours: resp.p99_ns ?? 0, theirs: openOha.latencyPercentiles.p99 * 1e9, unit: "ns", gate: true },
      { leg: "closed", metric: "rps", ours: s(closedOurs).rps, theirs: closedOha.summary.requestsPerSec, unit: "rps", gate: false },
      { leg: "closed", metric: "p50", ours: s(closedOurs).p50_ns ?? 0, theirs: closedOha.latencyPercentiles.p50 * 1e9, unit: "ns", gate: false },
      { leg: "closed", metric: "p99", ours: s(closedOurs).p99_ns ?? 0, theirs: closedOha.latencyPercentiles.p99 * 1e9, unit: "ns", gate: false }
    ];
    const agreementOk = printRows(rows);

    // ── Experiment B: client ceiling ──
    console.log(`\n${c.bold("━━ Experiment B: client ceiling (offered vs target rate) ━━")}`);
    let ceiling = 0;
    let wall: string | null = null;
    for (const rate of CEILING_RATES) {
      const cfg = structuredClone(baseConfig);
      cfg.benchmark.duration_per_endpoint = CEILING_DURATION;
      cfg.benchmark.load = { mode: "open", rate, max_in_flight: MAX_IN_FLIGHT };
      const cfgPath = join(runDir, `ceiling-${rate}.json`);
      writeFileSync(cfgPath, JSON.stringify(cfg, null, 2));
      const ours = runClient(bin, base, cfgPath, join(runDir, `ceiling-${rate}`));
      if (!ours.open) fail("ceiling run produced no open stats");
      const dropRatio = ours.open.dropped_iterations / Math.max(1, ours.open.attempted);
      const offeredRatio = ours.open.offered_rate / ours.open.target_rate;
      const line = `rate ${String(rate).padStart(6)}: offered ${ours.open.offered_rate.toFixed(0).padStart(7)} (${(offeredRatio * 100).toFixed(1)}%), dropped ${ours.open.dropped_iterations} (${(dropRatio * 100).toFixed(2)}%)`;
      if (dropRatio <= DROP_RATIO_OK && offeredRatio >= OFFERED_TRACK_OK) {
        console.log(`  ${c.green("✓")} ${line}`);
        ceiling = rate;
        continue;
      }
      console.log(`  ${c.red("✗")} ${line}`);
      // Attribute the wall: if oha sustains this rate against the same server,
      // the wall is the client's; if oha can't either, it's the server's.
      const attr = runOha(url, CEILING_DURATION, ["-q", String(rate), "-c", String(MAX_IN_FLIGHT), "--latency-correction"]);
      const ohaRatio = attr.summary.requestsPerSec / rate;
      wall = ohaRatio >= OFFERED_RATIO_OK ? "client" : "server";
      console.log(`    oha at the same rate: ${attr.summary.requestsPerSec.toFixed(0)} rps (${(ohaRatio * 100).toFixed(1)}%) → wall is the ${c.bold(wall)}'s`);
      break;
    }
    if (ceiling === CEILING_RATES[CEILING_RATES.length - 1]) {
      console.log(`  ceiling ≥ ${ceiling} req/s (max tested rate sustained)`);
    } else if (ceiling > 0) {
      console.log(`  last sustained rate: ${ceiling} req/s${wall ? ` (${wall} wall above it)` : ""}`);
    }

    console.log(`\n${c.bold("━━ Verdict ━━")}`);
    if (!agreementOk) {
      console.log(c.red("  calibration FAILED — open-mode drift beyond tolerance; investigate before trusting client v2 numbers"));
      process.exitCode = 1;
    } else {
      console.log(c.green("  calibration PASSED — open-mode RPS/p50/p99 agree with oha within tolerance"));
      console.log(c.dim(`  record the numbers + ceiling in docs/calibration.md (rates ≤ ~70% of the ceiling are trustworthy)`));
    }
  } finally {
    cleanup();
  }
}

try {
  await main();
} catch (err) {
  if (err instanceof CalibrationError) {
    console.error(`${c.red("✗")} ${err.message}`);
    process.exit(1);
  }
  throw err;
}
