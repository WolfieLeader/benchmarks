import { Router } from "@oak/oak";
import { INTERNAL_ERROR, INVALID_JSON_BODY, makeError, NOT_FOUND } from "../consts/errors.ts";
import { resolveRepository, type UserRepository } from "../database/repository.ts";
import { zCreateUser, zUpdateUser } from "../database/types.ts";

type DbState = { repository: UserRepository };

export const dbRoutes = new Router<DbState>();

dbRoutes.get("/:database/health", async (ctx) => {
  const repository = resolveRepository(ctx.params.database);
  if (!repository) {
    ctx.response.status = 503;
    ctx.response.body = "Service Unavailable";
    return;
  }

  const healthy = await repository.healthCheck().catch(() => false);
  if (healthy) {
    ctx.response.body = "OK";
    return;
  }
  ctx.response.status = 503;
  ctx.response.body = "Service Unavailable";
});

dbRoutes.use("/:database/:path*", async (ctx, next) => {
  const repository = resolveRepository(ctx.params.database);
  if (!repository) {
    ctx.response.status = 404;
    ctx.response.body = makeError(NOT_FOUND, `unknown database type: ${ctx.params.database}`);
    return;
  }
  ctx.state.repository = repository;
  await next();
});

dbRoutes.post("/:database/users", async (ctx) => {
  const { repository } = ctx.state;

  let body: unknown;
  try {
    body = await ctx.request.body.json();
  } catch (err) {
    ctx.response.status = 400;
    ctx.response.body = makeError(INVALID_JSON_BODY, err);
    return;
  }

  const parsed = zCreateUser.safeParse(body);
  if (!parsed.success) {
    ctx.response.status = 400;
    ctx.response.body = makeError(INVALID_JSON_BODY, parsed.error.message);
    return;
  }

  try {
    const user = await repository.create(parsed.data);
    ctx.response.status = 201;
    ctx.response.body = user;
  } catch (err) {
    ctx.response.status = 500;
    ctx.response.body = makeError(INTERNAL_ERROR, err);
  }
});

dbRoutes.get("/:database/users/:id", async (ctx) => {
  const { repository } = ctx.state;
  const { id } = ctx.params;

  try {
    const user = await repository.findById(id);
    if (!user) {
      ctx.response.status = 404;
      ctx.response.body = makeError(NOT_FOUND, `user with id ${id} not found`);
      return;
    }
    ctx.response.body = user;
  } catch (err) {
    ctx.response.status = 500;
    ctx.response.body = makeError(INTERNAL_ERROR, err);
  }
});

dbRoutes.patch("/:database/users/:id", async (ctx) => {
  const { repository } = ctx.state;
  const { id } = ctx.params;

  let body: unknown;
  try {
    body = await ctx.request.body.json();
  } catch (err) {
    ctx.response.status = 400;
    ctx.response.body = makeError(INVALID_JSON_BODY, err);
    return;
  }

  const parsed = zUpdateUser.safeParse(body);
  if (!parsed.success) {
    ctx.response.status = 400;
    ctx.response.body = makeError(INVALID_JSON_BODY, parsed.error.message);
    return;
  }

  try {
    const user = await repository.update(id, parsed.data);
    if (!user) {
      ctx.response.status = 404;
      ctx.response.body = makeError(NOT_FOUND, `user with id ${id} not found`);
      return;
    }
    ctx.response.body = user;
  } catch (err) {
    ctx.response.status = 500;
    ctx.response.body = makeError(INTERNAL_ERROR, err);
  }
});

dbRoutes.delete("/:database/users/:id", async (ctx) => {
  const { repository } = ctx.state;
  const { id } = ctx.params;

  try {
    const deleted = await repository.delete(id);
    if (!deleted) {
      ctx.response.status = 404;
      ctx.response.body = makeError(NOT_FOUND, `user with id ${id} not found`);
      return;
    }
    ctx.response.body = { success: true };
  } catch (err) {
    ctx.response.status = 500;
    ctx.response.body = makeError(INTERNAL_ERROR, err);
  }
});

dbRoutes.delete("/:database/users", async (ctx) => {
  const { repository } = ctx.state;

  try {
    await repository.deleteAll();
    ctx.response.body = { success: true };
  } catch (err) {
    ctx.response.status = 500;
    ctx.response.body = makeError(INTERNAL_ERROR, err);
  }
});

dbRoutes.delete("/:database/reset", async (ctx) => {
  const { repository } = ctx.state;

  try {
    await repository.deleteAll();
    ctx.response.body = { status: "ok" };
  } catch (err) {
    ctx.response.status = 500;
    ctx.response.body = makeError(INTERNAL_ERROR, err);
  }
});
