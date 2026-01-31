import { createApp } from "./app";
import { env } from "./config/env";
import { disconnectDatabases, initializeDatabases } from "./database/repository";

async function main() {
  await initializeDatabases();

  const app = await createApp();

  await app.listen({ port: env.PORT, host: env.HOST });

  console.log(`Server running at http://${env.HOST}:${env.PORT}/`);

  async function shutdown() {
    console.log("Shutting down...");
    await app.close();
    await disconnectDatabases();
    process.exit(0);
  }

  process.on("SIGTERM", shutdown);
  process.on("SIGINT", shutdown);
}

main();
