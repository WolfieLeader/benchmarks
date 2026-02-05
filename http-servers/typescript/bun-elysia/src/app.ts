import { Elysia } from "elysia";
import { env } from "./config/env";
import { INTERNAL_ERROR, INVALID_JSON_BODY, makeError, NOT_FOUND } from "./consts/errors";
import { dbRouter } from "./routes/db";
import { paramsRouter } from "./routes/params";

export function createApp() {
  const app = new Elysia();

  if (env.ENV !== "prod") {
    app
      .derive(({ request }) => ({ startTime: performance.now(), pathname: new URL(request.url).pathname }))
      .onRequest(({ request }) => {
        console.log(`<-- ${request.method} ${new URL(request.url).pathname}`);
      })
      .onAfterResponse(({ request, set, startTime, pathname }) => {
        const ms = (performance.now() - startTime).toFixed(0);
        console.log(`--> ${request.method} ${pathname} ${set.status || 200} ${ms}ms`);
      });
  }

  app.get("/", () => ({ hello: "world" }));
  app.get("/health", () => "OK");

  app.group("/params", (app) => app.use(paramsRouter));
  app.group("/db", (app) => app.use(dbRouter));

  app.onError(({ code, set, error }) => {
    if (code === "NOT_FOUND") {
      set.status = 404;
      return { error: NOT_FOUND };
    }
    if (code === "PARSE" || code === "VALIDATION") {
      set.status = 400;
      return makeError(INVALID_JSON_BODY, (error as Error).message);
    }
    set.status = 500;
    return makeError(INTERNAL_ERROR, (error as Error).message);
  });

  return app;
}
