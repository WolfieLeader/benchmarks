import { Application, Router } from "@oak/oak";
import { dbRoutes } from "./routes/db.ts";
import { paramsRoutes } from "./routes/params.ts";
import { env } from "./config/env.ts";
import { INTERNAL_ERROR, makeError, NOT_FOUND } from "./consts/errors.ts";

export function createApp() {
  const app = new Application();
  const router = new Router();

  app.use(async (ctx, next) => {
    try {
      await next();
    } catch (err) {
      if (err instanceof Error) {
        ctx.response.status = 500;
        ctx.response.body = makeError(INTERNAL_ERROR, err.message || undefined);
      }
    }
  });

  if (env.ENV !== "prod") {
    app.use(async (ctx, next) => {
      const start = Date.now();
      await next();
      const ms = Date.now() - start;
      console.log(
        `${ctx.request.method} ${ctx.request.url.pathname} ${ctx.response.status} ${ms}ms`,
      );
    });
  }

  router.get("/", (ctx) => {
    ctx.response.body = { message: "Hello World" };
  });

  router.get("/health", (ctx) => {
    ctx.response.body = { status: "healthy" };
  });

  router.use("/params", paramsRoutes.routes(), paramsRoutes.allowedMethods());
  router.use("/db", dbRoutes.routes(), dbRoutes.allowedMethods());

  app.use(router.routes());
  app.use(router.allowedMethods());

  app.use((ctx) => {
    if (ctx.response.status === 404 && ctx.response.body === undefined) {
      ctx.response.status = 404;
      ctx.response.body = { error: NOT_FOUND };
    }
  });

  return app;
}
