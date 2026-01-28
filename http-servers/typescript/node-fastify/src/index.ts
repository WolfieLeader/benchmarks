import { createApp } from "./app";
import { env } from "./config/env";

async function main() {
  const app = await createApp();

  await app.listen({ port: env.PORT, host: env.HOST });

  console.log(`Server running at http://${env.HOST}:${env.PORT}/`);
}

main();
