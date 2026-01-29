import { Elysia } from "elysia";
import { INTERNAL_ERROR, INVALID_JSON_BODY, NOT_FOUND } from "../consts/errors";
import { resolveRepository } from "../database/repository";
import { zCreateUser, zUpdateUser } from "../database/types";

class RepositoryNotFoundError extends Error {}

const withRepository = new Elysia()
  .error({ REPOSITORY_NOT_FOUND: RepositoryNotFoundError })
  .onError({ as: "scoped" }, ({ code, set }) => {
    if (code === "REPOSITORY_NOT_FOUND") {
      set.status = 404;
      return { error: NOT_FOUND };
    }
  })
  .derive({ as: "scoped" }, ({ params }) => {
    const { database } = params as { database: string };
    const repository = resolveRepository(database);
    if (!repository) throw new RepositoryNotFoundError();
    return { repository };
  });

export const dbRouter = new Elysia()
  .use(withRepository)
  .post("/:database/users", async ({ repository, body, set }) => {
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
  .get("/:database/users/:id", async ({ repository, params, set }) => {
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
  .patch("/:database/users/:id", async ({ repository, params, body, set }) => {
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
  .delete("/:database/users/:id", async ({ repository, params, set }) => {
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
  .delete("/:database/users", async ({ repository, set }) => {
    try {
      await repository.deleteAll();
      return { success: true };
    } catch {
      set.status = 500;
      return { error: INTERNAL_ERROR };
    }
  })
  .delete("/:database/reset", async ({ repository, set }) => {
    try {
      await repository.deleteAll();
      return { status: "ok" };
    } catch {
      set.status = 500;
      return { error: INTERNAL_ERROR };
    }
  })
  .get("/:database/health", async ({ repository, set }) => {
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
