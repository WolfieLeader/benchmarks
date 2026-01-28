import { Elysia } from "elysia";
import { INTERNAL_ERROR, INVALID_JSON_BODY, NOT_FOUND } from "../consts/errors";
import { resolveRepository } from "../database/repository";
import { zCreateUser, zUpdateUser } from "../database/types";

export const dbRouter = new Elysia()
  .post("/:database/users", async ({ params, body, set }) => {
    const repository = resolveRepository(params.database);
    if (!repository) {
      set.status = 404;
      return { error: NOT_FOUND };
    }

    const parsed = zCreateUser.safeParse(body);
    if (!parsed.success) {
      set.status = 400;
      return { error: INVALID_JSON_BODY };
    }

    try {
      const user = await repository.create(parsed.data);
      set.status = 201;
      return user;
    } catch {
      set.status = 500;
      return { error: INTERNAL_ERROR };
    }
  })
  .get("/:database/users/:id", async ({ params, set }) => {
    const repository = resolveRepository(params.database);
    if (!repository) {
      set.status = 404;
      return { error: NOT_FOUND };
    }

    try {
      const user = await repository.findById(params.id);
      if (!user) {
        set.status = 404;
        return { error: NOT_FOUND };
      }
      return user;
    } catch {
      set.status = 500;
      return { error: INTERNAL_ERROR };
    }
  })
  .patch("/:database/users/:id", async ({ params, body, set }) => {
    const repository = resolveRepository(params.database);
    if (!repository) {
      set.status = 404;
      return { error: NOT_FOUND };
    }

    const parsed = zUpdateUser.safeParse(body);
    if (!parsed.success) {
      set.status = 400;
      return { error: INVALID_JSON_BODY };
    }

    try {
      const user = await repository.update(params.id, parsed.data);
      if (!user) {
        set.status = 404;
        return { error: NOT_FOUND };
      }
      return user;
    } catch {
      set.status = 500;
      return { error: INTERNAL_ERROR };
    }
  })
  .delete("/:database/users/:id", async ({ params, set }) => {
    const repository = resolveRepository(params.database);
    if (!repository) {
      set.status = 404;
      return { error: NOT_FOUND };
    }

    try {
      const deleted = await repository.delete(params.id);
      if (!deleted) {
        set.status = 404;
        return { error: NOT_FOUND };
      }
      return { success: true };
    } catch {
      set.status = 500;
      return { error: INTERNAL_ERROR };
    }
  })
  .delete("/:database/users", async ({ params, set }) => {
    const repository = resolveRepository(params.database);
    if (!repository) {
      set.status = 404;
      return { error: NOT_FOUND };
    }

    try {
      await repository.deleteAll();
      return { success: true };
    } catch {
      set.status = 500;
      return { error: INTERNAL_ERROR };
    }
  })
  .get("/:database/health", async ({ params, set }) => {
    const repository = resolveRepository(params.database);
    if (!repository) {
      set.status = 404;
      return { error: NOT_FOUND };
    }

    try {
      const healthy = await repository.healthCheck();
      if (!healthy) {
        set.status = 503;
        return { error: "database unavailable" };
      }
      return { status: "healthy" };
    } catch {
      set.status = 500;
      return { error: INTERNAL_ERROR };
    }
  });
