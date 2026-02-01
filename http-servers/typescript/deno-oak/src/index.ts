import { createApp } from "./app.ts";
import { env } from "./config/env.ts";
import { disconnectDatabases, initializeDatabases } from "./database/repository.ts";

await initializeDatabases();

const app = createApp();
const controller = new AbortController();

const shutdown = async () => {
  console.log("Shutting down...");
  controller.abort();
  await disconnectDatabases();
  Deno.exit(0);
};

Deno.addSignalListener("SIGINT", shutdown);
Deno.addSignalListener("SIGTERM", shutdown);

console.log(`Server running at http://${env.HOST}:${env.PORT}/`);

await app.listen({
  hostname: env.HOST,
  port: env.PORT,
  signal: controller.signal
});
