import { Elysia } from "elysia";
import { paramsRouter } from "./routes/params";
import { env } from "./config/env";
import { NOT_FOUND, INTERNAL_ERROR } from "./consts/errors";

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

  app.get("/", () => "OK");
  app.get("/health", () => ({ message: "Hello World" }));

  app.group("/params", (app) => app.use(paramsRouter));

  app.onError(({ code, error }) => {
    if (code === "NOT_FOUND") return new Response(JSON.stringify({ error: NOT_FOUND }), { status: 404 });
    return new Response(JSON.stringify({ error: (error as Error).message || INTERNAL_ERROR }), { status: 500 });
  });

  return app;
}
