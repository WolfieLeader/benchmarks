import { createApp } from "./app.ts";
import { env } from "./config/env.ts";

const app = createApp();

console.log(`Server running at http://${env.HOST}:${env.PORT}/`);

await app.listen({ hostname: env.HOST, port: env.PORT });
