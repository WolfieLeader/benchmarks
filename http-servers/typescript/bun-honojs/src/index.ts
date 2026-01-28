import { createApp } from "./app";
import { env } from "./consts/env";

const app = createApp();

Bun.serve({
  port: env.PORT,
  hostname: env.HOST,
  fetch: app.fetch,
  maxRequestBodySize: 10 * 1024 * 1024 // 10 MB
});

console.log(`Server running at http://${env.HOST}:${env.PORT}/`);
