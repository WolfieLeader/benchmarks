import { Elysia } from "elysia";
import { env } from "./config/env";
import { INTERNAL_ERROR, makeError, NOT_FOUND } from "./consts/errors";
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

  app.get("/", () => ({ message: "Hello World" }));
  app.get("/health", () => ({ status: "healthy" }));

  app.group("/params", (app) => app.use(paramsRouter));
  app.group("/db", (app) => app.use(dbRouter));

  app.onError(({ code, error }) => {
    if (code === "NOT_FOUND") return new Response(JSON.stringify({ error: NOT_FOUND }), { status: 404 });
    const message = (error as Error).message || undefined;
    return new Response(JSON.stringify(makeError(INTERNAL_ERROR, message)), { status: 500 });
  });

  return app;
}
