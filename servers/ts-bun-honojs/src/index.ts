import {
  disconnectDatabases,
  env,
  initializeDatabases,
  MAX_REQUEST_BYTES,
  setIdGenerator,
  setRedisRepositoryFactory
} from "@bench/shared";
import { randomUUIDv7 } from "bun";
import { createApp } from "./app";
import { RedisUserRepository } from "./redis";

// Wire the Bun-native adapters (PLAN §3) before opening any DB connection:
// randomUUIDv7 for id generation, Bun.RedisClient-backed repository for Redis.
setIdGenerator(randomUUIDv7);
setRedisRepositoryFactory((url) => new RedisUserRepository(url));

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
