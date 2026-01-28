import { Router } from "express";
import { INTERNAL_ERROR, INVALID_JSON_BODY, NOT_FOUND } from "../consts/errors";
import { resolveRepository } from "../database/repository";
import { zCreateUser, zUpdateUser } from "../database/types";

export const dbRouter = Router();

dbRouter.post("/:database/users", async (req, res) => {
  const repository = resolveRepository(req.params.database);
  if (!repository) {
    res.status(404).json({ error: NOT_FOUND });
    return;
  }

  const parsed = zCreateUser.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: INVALID_JSON_BODY });
    return;
  }

  try {
    const user = await repository.create(parsed.data);
    res.status(201).json(user);
  } catch {
    res.status(500).json({ error: INTERNAL_ERROR });
  }
});

dbRouter.get("/:database/users/:id", async (req, res) => {
  const repository = resolveRepository(req.params.database);
  if (!repository) {
    res.status(404).json({ error: NOT_FOUND });
    return;
  }

  try {
    const user = await repository.findById(req.params.id);
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
  const repository = resolveRepository(req.params.database);
  if (!repository) {
    res.status(404).json({ error: NOT_FOUND });
    return;
  }

  const parsed = zUpdateUser.safeParse(req.body);
  if (!parsed.success) {
    res.status(400).json({ error: INVALID_JSON_BODY });
    return;
  }

  try {
    const user = await repository.update(req.params.id, parsed.data);
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
  const repository = resolveRepository(req.params.database);
  if (!repository) {
    res.status(404).json({ error: NOT_FOUND });
    return;
  }

  try {
    const deleted = await repository.delete(req.params.id);
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
  const repository = resolveRepository(req.params.database);
  if (!repository) {
    res.status(404).json({ error: NOT_FOUND });
    return;
  }

  try {
    await repository.deleteAll();
    res.json({ success: true });
  } catch {
    res.status(500).json({ error: INTERNAL_ERROR });
  }
});

dbRouter.get("/:database/health", async (req, res) => {
  const repository = resolveRepository(req.params.database);
  if (!repository) {
    res.status(404).json({ error: NOT_FOUND });
    return;
  }

  try {
    const healthy = await repository.healthCheck();
    if (!healthy) {
      res.status(503).json({ error: "database unavailable" });
      return;
    }
    res.json({ status: "healthy" });
  } catch {
    res.status(500).json({ error: INTERNAL_ERROR });
  }
});
