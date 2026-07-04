import { createApp } from "@bench/hono-app";
import { disconnectDatabases, env, initializeDatabases } from "@bench/shared";
import { serve } from "@hono/node-server";

// Node runtime entry (PLAN §4): the shared Hono app runs on Node via
// @hono/node-server, which bridges Hono's web-standard `fetch` handler onto a
// real `node:http` server. No adapter injection is needed — the default
// @bench/shared adapters (uuid v7 id generator, ioredis-backed Redis repository)
// are the portable ones; only the Bun entry swaps in its native equivalents.

await initializeDatabases();

const app = createApp();

const server = serve({ fetch: app.fetch, port: env.PORT, hostname: env.HOST }, () => {
  console.log(`Server running at http://${env.HOST}:${env.PORT}/`);
});

async function shutdown() {
  console.log("Shutting down...");
  // Stop accepting new connections and wait for in-flight requests to finish
  // before tearing down the databases they depend on (typescript.md rule 22).
  // Node >=19 (this runtime is 26) closes idle keep-alive sockets inside close()
  // itself, so close()'s callback resolves promptly. (@hono/node-server's
  // ServerType is a union including Http2SecureServer, which has no
  // closeIdleConnections — awaiting close() is the portable drain.)
  await new Promise<void>((resolve, reject) => {
    server.close((err) => (err ? reject(err) : resolve()));
  });
  await disconnectDatabases();
  process.exit(0);
}

process.on("SIGTERM", shutdown);
process.on("SIGINT", shutdown);
