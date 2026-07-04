import { createApp } from "@bench/hono-app";
import {
  disconnectDatabases,
  env,
  initializeDatabases,
  setIdGenerator,
  setRedisRepositoryFactory
} from "@bench/shared";
import { randomUUIDv7 } from "bun";
import { RedisUserRepository } from "./redis";

// Bun runtime entry (PLAN §4): runs the shared Hono app on Bun's native
// `Bun.serve`. Wire the Bun-native adapters (PLAN §3) before opening any DB
// connection — randomUUIDv7 for id generation, a Bun.RedisClient-backed
// repository for Redis — so the Bun entry keeps its native edge while sharing the
// same app + @bench/shared as the Node and Deno entries. The 10 MiB request cap
// lives in the shared app's `bodyLimit` middleware, identical across runtimes.
setIdGenerator(randomUUIDv7);
setRedisRepositoryFactory((url) => new RedisUserRepository(url));

await initializeDatabases();

const app = createApp();

const server = Bun.serve({
  port: env.PORT,
  hostname: env.HOST,
  fetch: app.fetch
});

console.log(`Server running at http://${env.HOST}:${env.PORT}/`);

async function shutdown() {
  console.log("Shutting down...");
  // Stop accepting new connections and drain in-flight requests before
  // disconnecting the databases they depend on.
  await server.stop();
  await disconnectDatabases();
  process.exit(0);
}

process.on("SIGTERM", shutdown);
process.on("SIGINT", shutdown);
