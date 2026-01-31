import { createApp } from "./app";
import { env } from "./config/env";
import { disconnectDatabases, initializeDatabases } from "./database/repository";

await initializeDatabases();

const app = createApp();

Bun.serve({
  port: env.PORT,
  hostname: env.HOST,
  fetch: app.fetch,
  maxRequestBodySize: 10 * 1024 * 1024
});

console.log(`Server running at http://${env.HOST}:${env.PORT}/`);

async function shutdown() {
  console.log("Shutting down...");
  await disconnectDatabases();
  process.exit(0);
}

process.on("SIGTERM", shutdown);
process.on("SIGINT", shutdown);
