import { Hono } from "hono";

const app = new Hono();

app.get("/", (c) => {
  return c.json({ message: "Hello World!" });
});

app.get("/ping", (c) => {
  return c.text("PONG!");
});

export default app;
