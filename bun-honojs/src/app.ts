import { Hono } from "hono";
import { logger } from "hono/logger";
import { paramsRoutes } from "./routes/params";

export function createApp() {
  const app = new Hono();

  app.use(logger());

  app.get("/", (c) => {
    return c.json({ message: "Hello World!" });
  });
  app.get("/ping", (c) => {
    return c.text("PONG!");
  });

  app.route("/params", paramsRoutes);

  app.onError((err, c) => {
    return c.json({ error: err.message }, 500);
  });

  return app;
}
