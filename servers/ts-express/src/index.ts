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
  // before tearing down the databases they depend on. closeIdleConnections
  // drops idle keep-alive sockets so close() can resolve promptly (Node >=19
  // already does this inside close(); the explicit call keeps the intent
  // visible and covers older runtimes).
  await new Promise<void>((resolve) => {
    server.close(() => resolve());
    server.closeIdleConnections();
  });
  await disconnectDatabases();
  process.exit(0);
}

process.on("SIGTERM", shutdown);
process.on("SIGINT", shutdown);
