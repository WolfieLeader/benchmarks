import {
  Body,
  Controller,
  Delete,
  Get,
  Header,
  HttpCode,
  HttpException,
  HttpStatus,
  Param,
  Patch,
  Post,
  Res
} from "@nestjs/common";
import type { Response } from "express";
import { INTERNAL_ERROR, INVALID_JSON_BODY, makeError, NOT_FOUND } from "../consts/errors";
import { resolveRepository, type UserRepository } from "./database/repository";
import { zCreateUser, zUpdateUser } from "./database/types";

@Controller("db")
export class DbController {
  private getRepo(database: string): UserRepository {
    const repository = resolveRepository(database);
    if (!repository) {
      throw new HttpException(makeError(NOT_FOUND, `unknown database type: ${database}`), HttpStatus.NOT_FOUND);
    }
    return repository;
  }

  private wrapError(err: unknown): never {
    if (err instanceof HttpException) throw err;
    throw new HttpException(makeError(INTERNAL_ERROR, err), HttpStatus.INTERNAL_SERVER_ERROR);
  }

  @Get(":database/health")
  @Header("Content-Type", "text/plain")
  async health(@Param("database") database: string, @Res() res: Response) {
    const repository = resolveRepository(database);
    if (!repository) {
      return res.status(503).send("Service Unavailable");
    }

    const healthy = await repository.healthCheck().catch(() => false);
    return res.status(healthy ? 200 : 503).send(healthy ? "OK" : "Service Unavailable");
  }

  @Post(":database/users")
  @HttpCode(201)
  async create(@Param("database") database: string, @Body() body: unknown) {
    const repository = this.getRepo(database);

    const parsed = zCreateUser.safeParse(body);
    if (!parsed.success) {
      throw new HttpException(makeError(INVALID_JSON_BODY, parsed.error.message), HttpStatus.BAD_REQUEST);
    }

    try {
      return await repository.create(parsed.data);
    } catch (err) {
      this.wrapError(err);
    }
  }

  @Get(":database/users/:id")
  async findById(@Param("database") database: string, @Param("id") id: string) {
    const repository = this.getRepo(database);

    try {
      const user = await repository.findById(id);
      if (!user) {
        throw new HttpException(makeError(NOT_FOUND, `user with id ${id} not found`), HttpStatus.NOT_FOUND);
      }
      return user;
    } catch (err) {
      this.wrapError(err);
    }
  }

  @Patch(":database/users/:id")
  async update(@Param("database") database: string, @Param("id") id: string, @Body() body: unknown) {
    const repository = this.getRepo(database);

    const parsed = zUpdateUser.safeParse(body);
    if (!parsed.success) {
      throw new HttpException(makeError(INVALID_JSON_BODY, parsed.error.message), HttpStatus.BAD_REQUEST);
    }

    try {
      const user = await repository.update(id, parsed.data);
      if (!user) {
        throw new HttpException(makeError(NOT_FOUND, `user with id ${id} not found`), HttpStatus.NOT_FOUND);
      }
      return user;
    } catch (err) {
      this.wrapError(err);
    }
  }

  @Delete(":database/users/:id")
  async deleteOne(@Param("database") database: string, @Param("id") id: string) {
    const repository = this.getRepo(database);

    try {
      const deleted = await repository.delete(id);
      if (!deleted) {
        throw new HttpException(makeError(NOT_FOUND, `user with id ${id} not found`), HttpStatus.NOT_FOUND);
      }
      return { success: true };
    } catch (err) {
      this.wrapError(err);
    }
  }

  @Delete(":database/users")
  async deleteAll(@Param("database") database: string) {
    const repository = this.getRepo(database);

    try {
      await repository.deleteAll();
      return { success: true };
    } catch (err) {
      this.wrapError(err);
    }
  }

  @Delete(":database/reset")
  async reset(@Param("database") database: string) {
    const repository = this.getRepo(database);

    try {
      await repository.deleteAll();
      return { status: "ok" };
    } catch (err) {
      this.wrapError(err);
    }
  }
}
