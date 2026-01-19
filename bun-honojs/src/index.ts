import { createApp } from "./app";
import { env } from "./env";

const app = createApp();

Bun.serve({
  port: env.PORT,
  hostname: env.HOST,
  fetch: app.fetch,
});

console.log(`🚀 Server running at http://${env.HOST}:${env.PORT}/`);
