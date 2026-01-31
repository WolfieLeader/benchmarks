import { createApp } from "./app";
import { env } from "./config/env";
import { disconnectDatabases, initializeDatabases } from "./database/repository";

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
