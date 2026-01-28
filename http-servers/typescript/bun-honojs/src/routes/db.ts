import { Hono } from "hono";
import { INTERNAL_ERROR, INVALID_JSON_BODY, NOT_FOUND } from "~/consts/errors";
import { resolveRepository, zCreateUser, zUpdateUser } from "~/database/repo";

export const dbRoutes = new Hono();

dbRoutes.post("/:database/users", async (c) => {
  const repository = resolveRepository(c.req.param("database"));
  if (!repository) {
    return c.json({ error: NOT_FOUND }, 404);
  }

  let body: unknown;
  try {
    body = await c.req.json();
  } catch {
    return c.json({ error: INVALID_JSON_BODY }, 400);
  }

  const parsed = zCreateUser.safeParse(body);
  if (!parsed.success) {
    return c.json({ error: INVALID_JSON_BODY }, 400);
  }

  try {
    const user = await repository.create(parsed.data);
    return c.json(user, 201);
  } catch {
    return c.json({ error: INTERNAL_ERROR }, 500);
  }
});

dbRoutes.get("/:database/users/:id", async (c) => {
  const repository = resolveRepository(c.req.param("database"));
  if (!repository) {
    return c.json({ error: NOT_FOUND }, 404);
  }

  try {
    const user = await repository.findById(c.req.param("id"));
    if (!user) {
      return c.json({ error: NOT_FOUND }, 404);
    }
    return c.json(user);
  } catch {
    return c.json({ error: INTERNAL_ERROR }, 500);
  }
});

dbRoutes.put("/:database/users/:id", async (c) => {
  const repository = resolveRepository(c.req.param("database"));
  if (!repository) {
    return c.json({ error: NOT_FOUND }, 404);
  }

  let body: unknown;
  try {
    body = await c.req.json();
  } catch {
    return c.json({ error: INVALID_JSON_BODY }, 400);
  }

  const parsed = zUpdateUser.safeParse(body);
  if (!parsed.success) {
    return c.json({ error: INVALID_JSON_BODY }, 400);
  }

  try {
    const user = await repository.update(c.req.param("id"), parsed.data);
    if (!user) {
      return c.json({ error: NOT_FOUND }, 404);
    }
    return c.json(user);
  } catch {
    return c.json({ error: INTERNAL_ERROR }, 500);
  }
});

dbRoutes.delete("/:database/users/:id", async (c) => {
  const repository = resolveRepository(c.req.param("database"));
  if (!repository) {
    return c.json({ error: NOT_FOUND }, 404);
  }

  try {
    const deleted = await repository.delete(c.req.param("id"));
    if (!deleted) {
      return c.json({ error: NOT_FOUND }, 404);
    }
    return c.json({ success: true });
  } catch {
    return c.json({ error: INTERNAL_ERROR }, 500);
  }
});
