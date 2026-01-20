import { Hono } from "hono";
import { logger } from "hono/logger";
import { paramsRoutes } from "./routes/params";
import { env } from "./env";

export function createApp() {
  const app = new Hono();

  if (env.ENV !== "prod") {
    app.use(logger());
  }

  app.get("/", (c) => c.json({ message: "Hello, World!" }));
  app.get("/health", (c) => c.text("OK"));

  app.route("/params", paramsRoutes);

  app.notFound((c) => c.json({ error: "not found" }, 404));
  app.onError((err, c) => c.json({ error: err.message || "internal error" }, 500));

  return app;
}
