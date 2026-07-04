import { disconnectDatabases, env, initializeDatabases } from "@bench/shared";
import { createApp } from "./app.ts";

await initializeDatabases();

const app = createApp();
const controller = new AbortController();

const listening = app.listen({
  hostname: env.HOST,
  port: env.PORT,
  signal: controller.signal
});

const shutdown = async () => {
  console.log("Shutting down...");
  // Abort stops accepting new requests; awaiting the listen promise waits for
  // the server to close before disconnecting the databases it depends on.
  controller.abort();
  await listening;
  await disconnectDatabases();
  Deno.exit(0);
};

Deno.addSignalListener("SIGINT", shutdown);
Deno.addSignalListener("SIGTERM", shutdown);

console.log(`Server running at http://${env.HOST}:${env.PORT}/`);

await listening;
