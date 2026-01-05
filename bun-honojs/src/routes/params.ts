import { Hono } from "hono";

export const paramsRoutes = new Hono();

paramsRoutes.get("/search", (c) => {
  const q = c.req.query("q") || "none";

  const limitStr = c.req.query("limit");
  const limit = limitStr ? parseInt(limitStr) : 10;

  return c.json({ q, limit });
});
