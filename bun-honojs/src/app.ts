import { Hono } from "hono";
import { logger } from "hono/logger";
import { paramsRoutes } from "./routes/params";
import { env } from "./config/env";
import { NOT_FOUND, INTERNAL_ERROR } from "./consts/errors";

export function createApp() {
  const app = new Hono();

  if (env.ENV !== "prod") {
    app.use(logger());
  }

  app.get("/", (c) => c.text("OK"));
  app.get("/health", (c) => c.json({ message: "Hello World" }));

  app.route("/params", paramsRoutes);

  app.notFound((c) => c.json({ error: NOT_FOUND }, 404));
  app.onError((err, c) => c.json({ error: err.message || INTERNAL_ERROR }, 500));

  return app;
}
