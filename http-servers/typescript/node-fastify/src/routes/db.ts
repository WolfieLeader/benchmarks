import type { FastifyPluginAsync, FastifyRequest } from "fastify";
import { INTERNAL_ERROR, INVALID_JSON_BODY, makeError, NOT_FOUND } from "../consts/errors.js";
import { resolveRepository, type UserRepository } from "../database/repository.js";
import { zCreateUser, zUpdateUser } from "../database/types.js";

declare module "fastify" {
  interface FastifyRequest {
    repository: UserRepository;
  }
}

export const dbRoutes: FastifyPluginAsync = async (fastify) => {
  // Health has its own semantics (unknown database -> 503, not 404), so it lives
  // outside the encapsulated CRUD context below and skips the repository guard.
  fastify.get<{ Params: { database: string } }>("/:database/health", async (request, reply) => {
    const repository = resolveRepository(request.params.database);
    const healthy = repository ? await repository.healthCheck().catch(() => false) : false;
    reply
      .code(healthy ? 200 : 503)
      .type("text/plain")
      .send(healthy ? "OK" : "Service Unavailable");
  });

  // CRUD routes: an encapsulated context whose preHandler resolves the
  // repository (unknown database -> 404). The hook must SEND the response to
  // short-circuit — returning a value from a hook does not stop the handler
  // from running, which previously left `request.repository` null (500).
  await fastify.register(async (crud) => {
    crud.decorateRequest("repository", null as unknown as UserRepository);

    crud.addHook("preHandler", async (request: FastifyRequest<{ Params: { database: string } }>, reply) => {
      const repository = resolveRepository(request.params.database);
      if (!repository) {
        return reply.code(404).send(makeError(NOT_FOUND, `unknown database type: ${request.params.database}`));
      }
      request.repository = repository;
    });

    crud.post<{ Params: { database: string } }>("/:database/users", async (request, reply) => {
      const parsed = zCreateUser.safeParse(request.body);
      if (!parsed.success) {
        reply.code(400);
        return makeError(INVALID_JSON_BODY, parsed.error.message);
      }

      try {
        const user = await request.repository.create(parsed.data);
        reply.code(201);
        return user;
      } catch (err) {
        reply.code(500);
        return makeError(INTERNAL_ERROR, err);
      }
    });

    crud.get<{ Params: { database: string; id: string } }>("/:database/users/:id", async (request, reply) => {
      try {
        const user = await request.repository.findById(request.params.id);
        if (!user) {
          reply.code(404);
          return makeError(NOT_FOUND, `user with id ${request.params.id} not found`);
        }
        return user;
      } catch (err) {
        reply.code(500);
        return makeError(INTERNAL_ERROR, err);
      }
    });

    crud.patch<{ Params: { database: string; id: string } }>("/:database/users/:id", async (request, reply) => {
      const parsed = zUpdateUser.safeParse(request.body);
      if (!parsed.success) {
        reply.code(400);
        return makeError(INVALID_JSON_BODY, parsed.error.message);
      }

      try {
        const user = await request.repository.update(request.params.id, parsed.data);
        if (!user) {
          reply.code(404);
          return makeError(NOT_FOUND, `user with id ${request.params.id} not found`);
        }
        return user;
      } catch (err) {
        reply.code(500);
        return makeError(INTERNAL_ERROR, err);
      }
    });

    crud.delete<{ Params: { database: string; id: string } }>("/:database/users/:id", async (request, reply) => {
      try {
        const deleted = await request.repository.delete(request.params.id);
        if (!deleted) {
          reply.code(404);
          return makeError(NOT_FOUND, `user with id ${request.params.id} not found`);
        }
        return { success: true };
      } catch (err) {
        reply.code(500);
        return makeError(INTERNAL_ERROR, err);
      }
    });

    crud.delete<{ Params: { database: string } }>("/:database/users", async (request, reply) => {
      try {
        await request.repository.deleteAll();
        return { success: true };
      } catch (err) {
        reply.code(500);
        return makeError(INTERNAL_ERROR, err);
      }
    });

    crud.delete<{ Params: { database: string } }>("/:database/reset", async (request, reply) => {
      try {
        await request.repository.deleteAll();
        return { status: "ok" };
      } catch (err) {
        reply.code(500);
        return makeError(INTERNAL_ERROR, err);
      }
    });
  });
};
