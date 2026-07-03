import { createApp } from "./app";
import { env } from "./config/env";
import { MAX_REQUEST_BYTES } from "./consts/defaults";
import { disconnectDatabases, initializeDatabases } from "./database/repository";

await initializeDatabases();

const app = createApp();

Bun.serve({
  port: env.PORT,
  hostname: env.HOST,
  fetch: app.fetch,
  maxRequestBodySize: MAX_REQUEST_BYTES
});

console.log(`Server running at http://${env.HOST}:${env.PORT}/`);

async function shutdown() {
  console.log("Shutting down...");
  await disconnectDatabases();
  process.exit(0);
}

process.on("SIGTERM", shutdown);
process.on("SIGINT", shutdown);
