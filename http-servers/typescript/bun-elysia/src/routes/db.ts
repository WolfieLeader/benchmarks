import { Elysia } from "elysia";
import { INTERNAL_ERROR, INVALID_JSON_BODY, makeError, NOT_FOUND } from "../consts/errors";
import { resolveRepository } from "../database/repository";
import { zCreateUser, zUpdateUser } from "../database/types";

class RepositoryNotFoundError extends Error {}

const withRepository = new Elysia()
  .error({ REPOSITORY_NOT_FOUND: RepositoryNotFoundError })
  .onError({ as: "scoped" }, ({ code, set, error }) => {
    if (code === "REPOSITORY_NOT_FOUND") {
      set.status = 404;
      return makeError(NOT_FOUND, error.message || "unknown database type");
    }
  })
  .derive({ as: "scoped" }, ({ params }) => {
    const { database } = params as { database: string };
    const repository = resolveRepository(database);
    if (!repository) throw new RepositoryNotFoundError(`unknown database type: ${database}`);
    return { repository };
  });

export const dbRouter = new Elysia()
  .use(withRepository)
  .post("/:database/users", async ({ repository, body, set }) => {
    const parsed = zCreateUser.safeParse(body);
    if (!parsed.success) {
      set.status = 400;
      return makeError(INVALID_JSON_BODY, parsed.error.message);
    }

    try {
      const user = await repository.create(parsed.data);
      set.status = 201;
      return user;
    } catch (err) {
      set.status = 500;
      return makeError(INTERNAL_ERROR, err);
    }
  })
  .get("/:database/users/:id", async ({ repository, params, set }) => {
    try {
      const user = await repository.findById(params.id);
      if (!user) {
        set.status = 404;
        return makeError(NOT_FOUND, `user with id ${params.id} not found`);
      }
      return user;
    } catch (err) {
      set.status = 500;
      return makeError(INTERNAL_ERROR, err);
    }
  })
  .patch("/:database/users/:id", async ({ repository, params, body, set }) => {
    const parsed = zUpdateUser.safeParse(body);
    if (!parsed.success) {
      set.status = 400;
      return makeError(INVALID_JSON_BODY, parsed.error.message);
    }

    try {
      const user = await repository.update(params.id, parsed.data);
      if (!user) {
        set.status = 404;
        return makeError(NOT_FOUND, `user with id ${params.id} not found`);
      }
      return user;
    } catch (err) {
      set.status = 500;
      return makeError(INTERNAL_ERROR, err);
    }
  })
  .delete("/:database/users/:id", async ({ repository, params, set }) => {
    try {
      const deleted = await repository.delete(params.id);
      if (!deleted) {
        set.status = 404;
        return makeError(NOT_FOUND, `user with id ${params.id} not found`);
      }
      return { success: true };
    } catch (err) {
      set.status = 500;
      return makeError(INTERNAL_ERROR, err);
    }
  })
  .delete("/:database/users", async ({ repository, set }) => {
    try {
      await repository.deleteAll();
      return { success: true };
    } catch (err) {
      set.status = 500;
      return makeError(INTERNAL_ERROR, err);
    }
  })
  .delete("/:database/reset", async ({ repository, set }) => {
    try {
      await repository.deleteAll();
      return { status: "ok" };
    } catch (err) {
      set.status = 500;
      return makeError(INTERNAL_ERROR, err);
    }
  });
