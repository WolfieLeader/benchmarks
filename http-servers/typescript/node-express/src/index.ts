import { createApp } from "./app.js";
import { env } from "./config/env.js";
import { disconnectDatabases, initializeDatabases } from "./database/repository.js";

await initializeDatabases();

const app = createApp();

const server = app.listen(env.PORT, env.HOST, () => {
  console.log(`Server running at http://${env.HOST}:${env.PORT}/`);
});

async function shutdown() {
  console.log("Shutting down...");
  server.close();
  await disconnectDatabases();
  process.exit(0);
}

process.on("SIGTERM", shutdown);
process.on("SIGINT", shutdown);
