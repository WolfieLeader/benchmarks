import type { MiddlewareHandler } from "hono";
import { Hono } from "hono";
import { INTERNAL_ERROR, INVALID_JSON_BODY, NOT_FOUND } from "../consts/errors";
import type { UserRepository } from "../database/repository";
import { resolveRepository } from "../database/repository";
import { zCreateUser, zUpdateUser } from "../database/types";

type DbVariables = {
  repository: UserRepository;
};

const withRepository: MiddlewareHandler<{ Variables: DbVariables }> = async (c, next) => {
  const database = c.req.param("database");
  if (!database) return c.json({ error: NOT_FOUND }, 404);

  const repository = resolveRepository(database);
  if (!repository) return c.json({ error: NOT_FOUND }, 404);

  c.set("repository", repository);
  await next();
};

export const dbRoutes = new Hono<{ Variables: DbVariables }>();

dbRoutes.use("/:database/*", withRepository);

// Create user
dbRoutes.post("/:database/users", async (c) => {
  const repository = c.get("repository");

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

// Read user
dbRoutes.get("/:database/users/:id", async (c) => {
  const repository = c.get("repository");

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

// Update user (PATCH - partial update)
dbRoutes.patch("/:database/users/:id", async (c) => {
  const repository = c.get("repository");

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

// Delete user
dbRoutes.delete("/:database/users/:id", async (c) => {
  const repository = c.get("repository");

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

// Reset - delete all users
dbRoutes.delete("/:database/users", async (c) => {
  const repository = c.get("repository");
  try {
    await repository.deleteAll();
    return c.json({ success: true });
  } catch {
    return c.json({ error: INTERNAL_ERROR }, 500);
  }
});

// Reset database - truncate all users
dbRoutes.delete("/:database/reset", async (c) => {
  const repository = c.get("repository");
  try {
    await repository.deleteAll();
    return c.json({ status: "ok" });
  } catch {
    return c.json({ error: INTERNAL_ERROR }, 500);
  }
});

// Health check
dbRoutes.get("/:database/health", async (c) => {
  const repository = c.get("repository");
  try {
    const healthy = await repository.healthCheck();
    return healthy ? c.json({ status: "healthy" }) : c.json({ error: "database unavailable" }, 503);
  } catch {
    return c.json({ error: INTERNAL_ERROR }, 500);
  }
});
