import { Hono } from "hono";
import { logger } from "hono/logger";
import { paramsRoutes } from "./routes/params";
import { cors } from "hono/cors";

export function createApp() {
  const app = new Hono();

  app.use(logger());
  app.use(
    cors({
      origin: "*",
      allowMethods: ["GET", "POST", "PUT", "DELETE", "OPTIONS"],
      allowHeaders: ["Content-Type", "Authorization", "X-Custom-Header"],
      credentials: true,
    }),
  );

  app.get("/", (c) => c.json({ message: "Hello, World!" }));
  app.get("/health", (c) => c.text("OK"));

  app.route("/params", paramsRoutes);

  app.onError((err, c) => c.json({ error: err.message || "internal error" }, 500));
  app.notFound((c) => c.json({ error: "not found" }, 404));

  return app;
}
