import express, { type Router } from "express";
import { INTERNAL_ERROR, INVALID_JSON_BODY, makeError, NOT_FOUND } from "../consts/errors";
import { resolveRepository, type UserRepository } from "../database/repository";
import { zCreateUser, zUpdateUser } from "../database/types";

declare global {
  namespace Express {
    interface Request {
      repository: UserRepository;
    }
  }
}

export const dbRouter: Router = express.Router();

dbRouter.get("/:database/health", async (req, res) => {
  const repository = resolveRepository(req.params.database);
  if (!repository) {
    res.status(503).send("Service Unavailable");
    return;
  }
  const healthy = await repository.healthCheck().catch(() => false);
  if (healthy) {
    res.send("OK");
    return;
  }
  res.status(503).send("Service Unavailable");
});

dbRouter.param("database", (req, res, next, database) => {
  const repository = resolveRepository(database);
  if (!repository) {
    res.status(404).json(makeError(NOT_FOUND, `unknown database type: ${database}`));
    return;
  }
  req.repository = repository;
  next();
});

dbRouter.post("/:database/users", async (req, res) => {
  const parsed = zCreateUser.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json(makeError(INVALID_JSON_BODY, parsed.error.message));
    return;
  }

  try {
    const user = await req.repository.create(parsed.data);
    res.status(201).json(user);
  } catch (err) {
    res.status(500).json(makeError(INTERNAL_ERROR, err));
  }
});

dbRouter.get("/:database/users/:id", async (req, res) => {
  try {
    const user = await req.repository.findById(req.params.id);
    if (!user) {
      res.status(404).json(makeError(NOT_FOUND, `user with id ${req.params.id} not found`));
      return;
    }
    res.json(user);
  } catch (err) {
    res.status(500).json(makeError(INTERNAL_ERROR, err));
  }
});

dbRouter.patch("/:database/users/:id", async (req, res) => {
  const parsed = zUpdateUser.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json(makeError(INVALID_JSON_BODY, parsed.error.message));
    return;
  }

  try {
    const user = await req.repository.update(req.params.id, parsed.data);
    if (!user) {
      res.status(404).json(makeError(NOT_FOUND, `user with id ${req.params.id} not found`));
      return;
    }
    res.json(user);
  } catch (err) {
    res.status(500).json(makeError(INTERNAL_ERROR, err));
  }
});

dbRouter.delete("/:database/users/:id", async (req, res) => {
  try {
    const deleted = await req.repository.delete(req.params.id);
    if (!deleted) {
      res.status(404).json(makeError(NOT_FOUND, `user with id ${req.params.id} not found`));
      return;
    }
    res.json({ success: true });
  } catch (err) {
    res.status(500).json(makeError(INTERNAL_ERROR, err));
  }
});

dbRouter.delete("/:database/users", async (req, res) => {
  try {
    await req.repository.deleteAll();
    res.json({ success: true });
  } catch (err) {
    res.status(500).json(makeError(INTERNAL_ERROR, err));
  }
});

dbRouter.delete("/:database/reset", async (req, res) => {
  try {
    await req.repository.deleteAll();
    res.json({ status: "ok" });
  } catch (err) {
    res.status(500).json(makeError(INTERNAL_ERROR, err));
  }
});
