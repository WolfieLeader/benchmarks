import type { MiddlewareHandler } from "hono";
import { Hono } from "hono";
import { INTERNAL_ERROR, INVALID_JSON_BODY, makeError, NOT_FOUND } from "../consts/errors";
import type { UserRepository } from "../database/repository";
import { resolveRepository } from "../database/repository";
import { zCreateUser, zUpdateUser } from "../database/types";

type DbVariables = {
  repository: UserRepository;
};

const withRepository: MiddlewareHandler<{ Variables: DbVariables }> = async (c, next) => {
  const database = c.req.param("database");
  if (!database) return c.json(makeError(NOT_FOUND, "database parameter missing"), 404);

  const repository = resolveRepository(database);
  if (!repository) return c.json(makeError(NOT_FOUND, `unknown database type: ${database}`), 404);

  c.set("repository", repository);
  await next();
};

export const dbRoutes = new Hono<{ Variables: DbVariables }>();

dbRoutes.get("/:database/health", async (c) => {
  const repository = resolveRepository(c.req.param("database"));
  if (!repository) return c.text("Service Unavailable", 503);

  const healthy = await repository.healthCheck().catch(() => false);
  return healthy ? c.text("OK") : c.text("Service Unavailable", 503);
});

dbRoutes.use("/:database/*", withRepository);

dbRoutes.post("/:database/users", async (c) => {
  const repository = c.get("repository");

  let body: unknown;
  try {
    body = await c.req.json();
  } catch (err) {
    return c.json(makeError(INVALID_JSON_BODY, err), 400);
  }

  const parsed = zCreateUser.safeParse(body);
  if (!parsed.success) {
    return c.json(makeError(INVALID_JSON_BODY, parsed.error.message), 400);
  }

  try {
    const user = await repository.create(parsed.data);
    return c.json(user, 201);
  } catch (err) {
    return c.json(makeError(INTERNAL_ERROR, err), 500);
  }
});

dbRoutes.get("/:database/users/:id", async (c) => {
  const repository = c.get("repository");
  const id = c.req.param("id");

  try {
    const user = await repository.findById(id);
    if (!user) {
      return c.json(makeError(NOT_FOUND, `user with id ${id} not found`), 404);
    }
    return c.json(user);
  } catch (err) {
    return c.json(makeError(INTERNAL_ERROR, err), 500);
  }
});

dbRoutes.patch("/:database/users/:id", async (c) => {
  const repository = c.get("repository");
  const id = c.req.param("id");

  let body: unknown;
  try {
    body = await c.req.json();
  } catch (err) {
    return c.json(makeError(INVALID_JSON_BODY, err), 400);
  }

  const parsed = zUpdateUser.safeParse(body);
  if (!parsed.success) {
    return c.json(makeError(INVALID_JSON_BODY, parsed.error.message), 400);
  }

  try {
    const user = await repository.update(id, parsed.data);
    if (!user) {
      return c.json(makeError(NOT_FOUND, `user with id ${id} not found`), 404);
    }
    return c.json(user);
  } catch (err) {
    return c.json(makeError(INTERNAL_ERROR, err), 500);
  }
});

dbRoutes.delete("/:database/users/:id", async (c) => {
  const repository = c.get("repository");
  const id = c.req.param("id");

  try {
    const deleted = await repository.delete(id);
    if (!deleted) {
      return c.json(makeError(NOT_FOUND, `user with id ${id} not found`), 404);
    }
    return c.json({ success: true });
  } catch (err) {
    return c.json(makeError(INTERNAL_ERROR, err), 500);
  }
});

dbRoutes.delete("/:database/users", async (c) => {
  const repository = c.get("repository");
  try {
    await repository.deleteAll();
    return c.json({ success: true });
  } catch (err) {
    return c.json(makeError(INTERNAL_ERROR, err), 500);
  }
});

dbRoutes.delete("/:database/reset", async (c) => {
  const repository = c.get("repository");
  try {
    await repository.deleteAll();
    return c.json({ status: "ok" });
  } catch (err) {
    return c.json(makeError(INTERNAL_ERROR, err), 500);
  }
});
