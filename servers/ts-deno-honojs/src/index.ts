import { createApp } from "@bench/hono-app";
import { disconnectDatabases, env, initializeDatabases } from "@bench/shared";

// Deno runtime entry (PLAN §4): the shared Hono app runs on Deno via the
// built-in `Deno.serve`, which takes Hono's web-standard `fetch` handler
// directly — the officially blessed Hono-on-Deno pattern. No adapter injection
// is needed; the default @bench/shared adapters (uuid v7, ioredis Redis
// repository) are the portable ones the Bun entry overrides with native bits.

await initializeDatabases();

const app = createApp();
const controller = new AbortController();

const server = Deno.serve(
  {
    hostname: env.HOST,
    port: env.PORT,
    signal: controller.signal,
    onListen: () => console.log(`Server running at http://${env.HOST}:${env.PORT}/`)
  },
  app.fetch
);

const shutdown = async () => {
  console.log("Shutting down...");
  // Abort stops accepting new connections; awaiting `finished` waits for the
  // server to drain in-flight requests before disconnecting the databases it
  // depends on (typescript.md rule 25).
  controller.abort();
  await server.finished;
  await disconnectDatabases();
  Deno.exit(0);
};

Deno.addSignalListener("SIGINT", shutdown);
Deno.addSignalListener("SIGTERM", shutdown);

await server.finished;
