import type { FastifyPluginAsync, FastifyRequest } from "fastify";
import { INTERNAL_ERROR, INVALID_JSON_BODY, NOT_FOUND, makeError } from "../consts/errors";
import { type UserRepository, resolveRepository } from "../database/repository";
import { zCreateUser, zUpdateUser } from "../database/types";

declare module "fastify" {
  interface FastifyRequest {
    repository: UserRepository;
  }
}

export const dbRoutes: FastifyPluginAsync = async (fastify) => {
  fastify.decorateRequest("repository", null as unknown as UserRepository);

  fastify.addHook("preHandler", async (request: FastifyRequest<{ Params: { database: string } }>, reply) => {
    const repository = resolveRepository(request.params.database);
    if (!repository) {
      reply.code(404);
      return reply.send(makeError(NOT_FOUND, `unknown database type: ${request.params.database}`));
    }
    request.repository = repository;
  });

  fastify.post<{ Params: { database: string } }>("/:database/users", async (request, reply) => {
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

  fastify.get<{ Params: { database: string; id: string } }>("/:database/users/:id", async (request, reply) => {
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

  fastify.patch<{ Params: { database: string; id: string } }>("/:database/users/:id", async (request, reply) => {
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

  fastify.delete<{ Params: { database: string; id: string } }>("/:database/users/:id", async (request, reply) => {
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

  fastify.delete<{ Params: { database: string } }>("/:database/users", async (request, reply) => {
    try {
      await request.repository.deleteAll();
      return { success: true };
    } catch (err) {
      reply.code(500);
      return makeError(INTERNAL_ERROR, err);
    }
  });

  fastify.delete<{ Params: { database: string } }>("/:database/reset", async (request, reply) => {
    try {
      await request.repository.deleteAll();
      return { status: "ok" };
    } catch (err) {
      reply.code(500);
      return makeError(INTERNAL_ERROR, err);
    }
  });

  fastify.get<{ Params: { database: string } }>("/:database/health", async (request, reply) => {
    try {
      const healthy = await request.repository.healthCheck();
      if (!healthy) {
        reply.code(503);
        return makeError("database unavailable", "health check returned false");
      }
      return { status: "healthy" };
    } catch (err) {
      reply.code(500);
      return makeError(INTERNAL_ERROR, err);
    }
  });
};
