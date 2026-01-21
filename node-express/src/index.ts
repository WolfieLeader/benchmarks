import { createApp } from "./app";
import { env } from "./config/env";

const app = createApp();

app.listen(env.PORT, env.HOST, () => {
  console.log(`Server running at http://${env.HOST}:${env.PORT}/`);
});
