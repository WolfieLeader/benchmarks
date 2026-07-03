import { env, INTERNAL_ERROR, INVALID_JSON_BODY, makeError, NOT_FOUND } from "@bench/shared";
import { Elysia } from "elysia";
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

  // Registered before the routes so it also catches body PARSE errors on grouped
  // routes: Elysia only applies onError to routes defined after it. The db
  // router's scoped REPOSITORY_NOT_FOUND returns undefined so its own scoped
  // handler still runs; any other unknown code gets the contract's 500 JSON
  // shape instead of Elysia's plain-text default.
  app.onError(({ code, set, error }) => {
    if (code === "NOT_FOUND") {
      set.status = 404;
      return { error: NOT_FOUND };
    }
    if (code === "PARSE" || code === "VALIDATION") {
      set.status = 400;
      return makeError(INVALID_JSON_BODY, (error as Error).message);
    }
    // REPOSITORY_NOT_FOUND is registered on the db router's scoped instance, so
    // this root handler's `code` union does not include it at the type level —
    // compare via String() without narrowing to defer to the scoped handler.
    if (String(code) === "REPOSITORY_NOT_FOUND") return;
    set.status = 500;
    return makeError(INTERNAL_ERROR, (error as Error).message);
  });

  app.get("/", () => ({ hello: "world" }));
  app.get("/health", () => "OK");

  app.group("/params", (app) => app.use(paramsRouter));
  app.group("/db", (app) => app.use(dbRouter));

  return app;
}
