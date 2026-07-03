import { disconnectDatabases, env, initializeDatabases } from "@bench/shared";
import { createApp } from "./app.js";

await initializeDatabases();

const app = createApp();

const server = app.listen(env.PORT, env.HOST, () => {
  console.log(`Server running at http://${env.HOST}:${env.PORT}/`);
});

async function shutdown() {
  console.log("Shutting down...");
  // Stop accepting new connections and wait for in-flight requests to finish
  // before tearing down the databases they depend on.
  await new Promise<void>((resolve) => server.close(() => resolve()));
  await disconnectDatabases();
  process.exit(0);
}

process.on("SIGTERM", shutdown);
process.on("SIGINT", shutdown);
