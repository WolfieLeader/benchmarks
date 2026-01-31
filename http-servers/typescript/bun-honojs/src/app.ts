import { Hono } from "hono";
import { logger } from "hono/logger";
import { env } from "./config/env";
import { INTERNAL_ERROR, makeError, NOT_FOUND } from "./consts/errors";
import { getAllDatabaseStatuses } from "./database/repository";
import { dbRoutes } from "./routes/db";
import { paramsRoutes } from "./routes/params";

export function createApp() {
  const app = new Hono();

  if (env.ENV !== "prod") {
    app.use(logger());
  }

  app.get("/", (c) => c.text("OK"));
  app.get("/health", async (c) => {
    const databases = await getAllDatabaseStatuses();
    return c.json({ status: "healthy", databases });
  });

  app.route("/db", dbRoutes);
  app.route("/params", paramsRoutes);

  app.notFound((c) => c.json({ error: NOT_FOUND }, 404));
  app.onError((err, c) => {
    return c.json(makeError(INTERNAL_ERROR, err), 500);
  });

  return app;
}
