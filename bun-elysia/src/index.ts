import { createApp } from "./app";
import { env } from "./config/env";

const app = createApp();

console.log(`Server running at http://${env.HOST}:${env.PORT}/`);

Bun.serve({
  port: env.PORT,
  hostname: env.HOST,
  fetch: app.fetch,
  maxRequestBodySize: 10 * 1024 * 1024,
});
