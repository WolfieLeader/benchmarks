import type { FastifyPluginAsync } from "fastify";
import { INTERNAL_ERROR, INVALID_JSON_BODY, NOT_FOUND } from "../consts/errors";
import { resolveRepository } from "../database/repository";
import { zCreateUser, zUpdateUser } from "../database/types";

export const dbRoutes: FastifyPluginAsync = async (fastify) => {
  fastify.post<{ Params: { database: string } }>("/:database/users", async (request, reply) => {
    const repository = resolveRepository(request.params.database);
    if (!repository) {
      reply.code(404);
      return { error: NOT_FOUND };
    }

    const parsed = zCreateUser.safeParse(request.body);
    if (!parsed.success) {
      reply.code(400);
      return { error: INVALID_JSON_BODY };
    }

    try {
      const user = await repository.create(parsed.data);
      reply.code(201);
      return user;
    } catch {
      reply.code(500);
      return { error: INTERNAL_ERROR };
    }
  });

  fastify.get<{ Params: { database: string; id: string } }>("/:database/users/:id", async (request, reply) => {
    const repository = resolveRepository(request.params.database);
    if (!repository) {
      reply.code(404);
      return { error: NOT_FOUND };
    }

    try {
      const user = await repository.findById(request.params.id);
      if (!user) {
        reply.code(404);
        return { error: NOT_FOUND };
      }
      return user;
    } catch {
      reply.code(500);
      return { error: INTERNAL_ERROR };
    }
  });

  fastify.patch<{ Params: { database: string; id: string } }>("/:database/users/:id", async (request, reply) => {
    const repository = resolveRepository(request.params.database);
    if (!repository) {
      reply.code(404);
      return { error: NOT_FOUND };
    }

    const parsed = zUpdateUser.safeParse(request.body);
    if (!parsed.success) {
      reply.code(400);
      return { error: INVALID_JSON_BODY };
    }

    try {
      const user = await repository.update(request.params.id, parsed.data);
      if (!user) {
        reply.code(404);
        return { error: NOT_FOUND };
      }
      return user;
    } catch {
      reply.code(500);
      return { error: INTERNAL_ERROR };
    }
  });

  fastify.delete<{ Params: { database: string; id: string } }>("/:database/users/:id", async (request, reply) => {
    const repository = resolveRepository(request.params.database);
    if (!repository) {
      reply.code(404);
      return { error: NOT_FOUND };
    }

    try {
      const deleted = await repository.delete(request.params.id);
      if (!deleted) {
        reply.code(404);
        return { error: NOT_FOUND };
      }
      return { success: true };
    } catch {
      reply.code(500);
      return { error: INTERNAL_ERROR };
    }
  });

  fastify.delete<{ Params: { database: string } }>("/:database/users", async (request, reply) => {
    const repository = resolveRepository(request.params.database);
    if (!repository) {
      reply.code(404);
      return { error: NOT_FOUND };
    }

    try {
      await repository.deleteAll();
      return { success: true };
    } catch {
      reply.code(500);
      return { error: INTERNAL_ERROR };
    }
  });

  fastify.get<{ Params: { database: string } }>("/:database/health", async (request, reply) => {
    const repository = resolveRepository(request.params.database);
    if (!repository) {
      reply.code(404);
      return { error: NOT_FOUND };
    }

    try {
      const healthy = await repository.healthCheck();
      if (!healthy) {
        reply.code(503);
        return { error: "database unavailable" };
      }
      return { status: "healthy" };
    } catch {
      reply.code(500);
      return { error: INTERNAL_ERROR };
    }
  });
};
