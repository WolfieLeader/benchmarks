import type { RequestHandler } from "express";
import { Router } from "express";
import { INTERNAL_ERROR, INVALID_JSON_BODY, NOT_FOUND } from "../consts/errors";
import { type UserRepository, resolveRepository } from "../database/repository";
import { zCreateUser, zUpdateUser } from "../database/types";

declare global {
  namespace Express {
    interface Request {
      repository: UserRepository;
    }
  }
}

const withRepository: RequestHandler<{ database: string }> = (req, res, next) => {
  const repository = resolveRepository(req.params.database);
  if (!repository) {
    res.status(404).json({ error: NOT_FOUND });
    return;
  }
  req.repository = repository;
  next();
};

export const dbRouter = Router();

// Apply middleware to all routes with :database param
dbRouter.param("database", (req, res, next, database) => {
  const repository = resolveRepository(database);
  if (!repository) {
    res.status(404).json({ error: NOT_FOUND });
    return;
  }
  req.repository = repository;
  next();
});

dbRouter.post("/:database/users", async (req, res) => {
  const parsed = zCreateUser.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: INVALID_JSON_BODY });
    return;
  }

  try {
    const user = await req.repository.create(parsed.data);
    res.status(201).json(user);
  } catch {
    res.status(500).json({ error: INTERNAL_ERROR });
  }
});

dbRouter.get("/:database/users/:id", async (req, res) => {
  try {
    const user = await req.repository.findById(req.params.id);
    if (!user) {
      res.status(404).json({ error: NOT_FOUND });
      return;
    }
    res.json(user);
  } catch {
    res.status(500).json({ error: INTERNAL_ERROR });
  }
});

dbRouter.patch("/:database/users/:id", async (req, res) => {
  const parsed = zUpdateUser.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: INVALID_JSON_BODY });
    return;
  }

  try {
    const user = await req.repository.update(req.params.id, parsed.data);
    if (!user) {
      res.status(404).json({ error: NOT_FOUND });
      return;
    }
    res.json(user);
  } catch {
    res.status(500).json({ error: INTERNAL_ERROR });
  }
});

dbRouter.delete("/:database/users/:id", async (req, res) => {
  try {
    const deleted = await req.repository.delete(req.params.id);
    if (!deleted) {
      res.status(404).json({ error: NOT_FOUND });
      return;
    }
    res.json({ success: true });
  } catch {
    res.status(500).json({ error: INTERNAL_ERROR });
  }
});

dbRouter.delete("/:database/users", async (req, res) => {
  try {
    await req.repository.deleteAll();
    res.json({ success: true });
  } catch {
    res.status(500).json({ error: INTERNAL_ERROR });
  }
});

dbRouter.get("/:database/health", async (req, res) => {
  try {
    const healthy = await req.repository.healthCheck();
    if (!healthy) {
      res.status(503).json({ error: "database unavailable" });
      return;
    }
    res.json({ status: "healthy" });
  } catch {
    res.status(500).json({ error: INTERNAL_ERROR });
  }
});
