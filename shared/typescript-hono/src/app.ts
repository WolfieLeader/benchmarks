// The single Hono application: root/health routes, the /db and /params route
// groups, the shared 10 MiB request cap, and the not-found/error shapes. The
// three per-runtime entries (ts-honojs, ts-bun-honojs, ts-deno-honojs) each
// import createApp and bind it to their runtime's server.

import { env, FILE_SIZE_EXCEEDS, INTERNAL_ERROR, makeError, MAX_REQUEST_BYTES, NOT_FOUND } from "@bench/shared";
import { Hono } from "hono";
import { bodyLimit } from "hono/body-limit";
import { logger } from "hono/logger";
import { createDbRoutes } from "./db-routes.ts";
import { createParamsRoutes } from "./params-routes.ts";

// The 10 MiB request cap (MAX_REQUEST_BYTES) is wired through Hono's own
// framework knob — the `bodyLimit` middleware — so every runtime enforces it
// identically (typescript.md rule 26). It lives here, in the shared app, rather
// than at each runtime's server layer, because @hono/node-server and Deno.serve
// expose no body-size option; putting it in the app is the only wiring that is
// both uniform across the three runtimes and idiomatic to Hono.
export function createApp(): Hono {
  const app = new Hono();

  if (env.ENV !== "prod") {
    app.use(logger());
  }

  app.use(
    bodyLimit({
      maxSize: MAX_REQUEST_BYTES,
      onError: (c) => c.json(makeError(FILE_SIZE_EXCEEDS, `request body exceeds limit ${MAX_REQUEST_BYTES}`), 413)
    })
  );

  app.get("/", (c) => c.json({ hello: "world" }));
  app.get("/health", (c) => c.text("OK"));

  app.route("/db", createDbRoutes());
  app.route("/params", createParamsRoutes());

  app.notFound((c) => c.json({ error: NOT_FOUND }, 404));
  app.onError((err, c) => {
    return c.json(makeError(INTERNAL_ERROR, err), 500);
  });

  return app;
}
