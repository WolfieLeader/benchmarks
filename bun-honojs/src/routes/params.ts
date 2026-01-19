import { Hono } from "hono";
import { getCookie, setCookie } from "hono/cookie";

const MAX_FILE_BYTES = 1 << 20; // 1MB
const SNIFF_LEN = 512;
const NULL_BYTE = 0x00;

export const paramsRoutes = new Hono();

paramsRoutes.get("/search", (c) => {
  const q = c.req.query("q") ?? "none";

  const limitStr = c.req.query("limit");
  const limitNum = Number(limitStr);
  const limit = !limitStr?.includes(".") && Number.isSafeInteger(limitNum) ? limitNum : 10;

  return c.json({ search: q, limit });
});

paramsRoutes.get("/url/:dynamic", (c) => {
  const dynamic = c.req.param("dynamic");
  return c.json({ dynamic });
});

paramsRoutes.get("/header", (c) => {
  const header = c.req.header("X-Custom-Header") ?? "none";
  return c.json({ header });
});

paramsRoutes.post("/body", async (c) => {
  let body: Record<string, any>;
  try {
    body = await c.req.json();
  } catch {
    return c.json({ error: "invalid JSON body" }, 400);
  }

  if (typeof body !== "object" || body === null || Array.isArray(body)) {
    return c.json({ error: "invalid JSON body" }, 400);
  }

  return c.json({ body });
});

paramsRoutes.get("/cookie", (c) => {
  const cookie = getCookie(c, "foo") ?? "none";
  setCookie(c, "bar", "12345", { maxAge: 10, httpOnly: true, path: "/" });
  return c.json({ cookie });
});

paramsRoutes.post("/form", async (c) => {
  let form: Record<string, string | File>;
  try {
    form = await c.req.parseBody();
  } catch {
    return c.json({ error: "invalid form data" }, 400);
  }

  if (typeof form !== "object" || form === null || Array.isArray(form)) {
    return c.json({ error: "invalid form data" }, 400);
  }

  const name = typeof form.name === "string" && form.name.trim() !== "" ? form.name : "none";

  const ageStr = typeof form.age === "string" ? form.age : "0";
  const ageNum = Number(ageStr);
  const age = !ageStr.includes(".") && Number.isSafeInteger(ageNum) ? ageNum : 0;

  return c.json({ name, age });
});

paramsRoutes.post("/file", async (c) => {
  let form: FormData;
  try {
    form = await c.req.formData();
  } catch {
    return c.json({ error: "invalid multipart form data" }, 400);
  }

  const file = form.get("file");
  if (!(file instanceof File)) {
    return c.json({ error: "file not found in form data" }, 400);
  }
  if (file.size > MAX_FILE_BYTES) {
    return c.json({ error: "file size exceeds limit" }, 413);
  }

  const buffer = await file.arrayBuffer();
  const data = new Uint8Array(buffer);
  if (data.length > MAX_FILE_BYTES) {
    return c.json({ error: "file size exceeds limit" }, 413);
  }

  const head = data.slice(0, SNIFF_LEN);
  if (head.includes(NULL_BYTE)) {
    return c.json({ error: "file does not look like plain text" }, 415);
  }

  if (!file.type || !file.type.startsWith("text/plain")) {
    return c.json({ error: "only text/plain files are allowed" }, 415);
  }

  if (data.includes(NULL_BYTE)) {
    return c.json({ error: "file does not look like plain text" }, 415);
  }

  let content: string;
  try {
    content = new TextDecoder("utf-8", { fatal: true }).decode(data);
  } catch {
    return c.json({ error: "file does not look like plain text" }, 415);
  }

  return c.json({
    filename: file.name,
    size: data.length,
    content,
  });
});
