import { Router } from "@oak/oak";
import {
  INTERNAL_ERROR,
  INVALID_JSON_BODY,
  NOT_FOUND,
} from "../consts/errors.ts";
import { resolveRepository } from "../database/repository.ts";
import { zCreateUser, zUpdateUser } from "../database/types.ts";

export const dbRoutes = new Router();

dbRoutes.post("/:database/users", async (ctx) => {
  const repository = resolveRepository(ctx.params.database);
  if (!repository) {
    ctx.response.status = 404;
    ctx.response.body = { error: NOT_FOUND };
    return;
  }

  let body: unknown;
  try {
    body = await ctx.request.body.json();
  } catch {
    ctx.response.status = 400;
    ctx.response.body = { error: INVALID_JSON_BODY };
    return;
  }

  const parsed = zCreateUser.safeParse(body);
  if (!parsed.success) {
    ctx.response.status = 400;
    ctx.response.body = { error: INVALID_JSON_BODY };
    return;
  }

  try {
    const user = await repository.create(parsed.data);
    ctx.response.status = 201;
    ctx.response.body = user;
  } catch {
    ctx.response.status = 500;
    ctx.response.body = { error: INTERNAL_ERROR };
  }
});

dbRoutes.get("/:database/users/:id", async (ctx) => {
  const repository = resolveRepository(ctx.params.database);
  if (!repository) {
    ctx.response.status = 404;
    ctx.response.body = { error: NOT_FOUND };
    return;
  }

  try {
    const user = await repository.findById(ctx.params.id);
    if (!user) {
      ctx.response.status = 404;
      ctx.response.body = { error: NOT_FOUND };
      return;
    }
    ctx.response.body = user;
  } catch {
    ctx.response.status = 500;
    ctx.response.body = { error: INTERNAL_ERROR };
  }
});

dbRoutes.patch("/:database/users/:id", async (ctx) => {
  const repository = resolveRepository(ctx.params.database);
  if (!repository) {
    ctx.response.status = 404;
    ctx.response.body = { error: NOT_FOUND };
    return;
  }

  let body: unknown;
  try {
    body = await ctx.request.body.json();
  } catch {
    ctx.response.status = 400;
    ctx.response.body = { error: INVALID_JSON_BODY };
    return;
  }

  const parsed = zUpdateUser.safeParse(body);
  if (!parsed.success) {
    ctx.response.status = 400;
    ctx.response.body = { error: INVALID_JSON_BODY };
    return;
  }

  try {
    const user = await repository.update(ctx.params.id, parsed.data);
    if (!user) {
      ctx.response.status = 404;
      ctx.response.body = { error: NOT_FOUND };
      return;
    }
    ctx.response.body = user;
  } catch {
    ctx.response.status = 500;
    ctx.response.body = { error: INTERNAL_ERROR };
  }
});

dbRoutes.delete("/:database/users/:id", async (ctx) => {
  const repository = resolveRepository(ctx.params.database);
  if (!repository) {
    ctx.response.status = 404;
    ctx.response.body = { error: NOT_FOUND };
    return;
  }

  try {
    const deleted = await repository.delete(ctx.params.id);
    if (!deleted) {
      ctx.response.status = 404;
      ctx.response.body = { error: NOT_FOUND };
      return;
    }
    ctx.response.body = { success: true };
  } catch {
    ctx.response.status = 500;
    ctx.response.body = { error: INTERNAL_ERROR };
  }
});

dbRoutes.delete("/:database/users", async (ctx) => {
  const repository = resolveRepository(ctx.params.database);
  if (!repository) {
    ctx.response.status = 404;
    ctx.response.body = { error: NOT_FOUND };
    return;
  }

  try {
    await repository.deleteAll();
    ctx.response.body = { success: true };
  } catch {
    ctx.response.status = 500;
    ctx.response.body = { error: INTERNAL_ERROR };
  }
});

dbRoutes.get("/:database/health", async (ctx) => {
  const repository = resolveRepository(ctx.params.database);
  if (!repository) {
    ctx.response.status = 404;
    ctx.response.body = { error: NOT_FOUND };
    return;
  }

  try {
    const healthy = await repository.healthCheck();
    if (!healthy) {
      ctx.response.status = 503;
      ctx.response.body = { error: "database unavailable" };
      return;
    }
    ctx.response.body = { status: "healthy" };
  } catch {
    ctx.response.status = 500;
    ctx.response.body = { error: INTERNAL_ERROR };
  }
});
