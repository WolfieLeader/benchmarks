import { Body, Controller, Delete, Get, HttpCode, HttpException, HttpStatus, Param, Patch, Post } from "@nestjs/common";
import { INTERNAL_ERROR, INVALID_JSON_BODY, makeError, NOT_FOUND } from "../consts/errors";
import { resolveRepository } from "./database/repository";
import { zCreateUser, zUpdateUser } from "./database/types";

@Controller("db")
export class DbController {
  @Post(":database/users")
  @HttpCode(201)
  async create(@Param("database") database: string, @Body() body: unknown) {
    const repository = resolveRepository(database);
    if (!repository) {
      throw new HttpException(makeError(NOT_FOUND, `unknown database type: ${database}`), HttpStatus.NOT_FOUND);
    }

    const parsed = zCreateUser.safeParse(body);
    if (!parsed.success) {
      throw new HttpException(makeError(INVALID_JSON_BODY, parsed.error.message), HttpStatus.BAD_REQUEST);
    }

    try {
      return await repository.create(parsed.data);
    } catch (err) {
      if (err instanceof HttpException) throw err;
      throw new HttpException(makeError(INTERNAL_ERROR, err), HttpStatus.INTERNAL_SERVER_ERROR);
    }
  }

  @Get(":database/users/:id")
  async findById(@Param("database") database: string, @Param("id") id: string) {
    const repository = resolveRepository(database);
    if (!repository) {
      throw new HttpException(makeError(NOT_FOUND, `unknown database type: ${database}`), HttpStatus.NOT_FOUND);
    }

    try {
      const user = await repository.findById(id);
      if (!user) {
        throw new HttpException(makeError(NOT_FOUND, `user with id ${id} not found`), HttpStatus.NOT_FOUND);
      }
      return user;
    } catch (err) {
      if (err instanceof HttpException) throw err;
      throw new HttpException(makeError(INTERNAL_ERROR, err), HttpStatus.INTERNAL_SERVER_ERROR);
    }
  }

  @Patch(":database/users/:id")
  @HttpCode(200)
  async update(@Param("database") database: string, @Param("id") id: string, @Body() body: unknown) {
    const repository = resolveRepository(database);
    if (!repository) {
      throw new HttpException(makeError(NOT_FOUND, `unknown database type: ${database}`), HttpStatus.NOT_FOUND);
    }

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
      if (err instanceof HttpException) throw err;
      throw new HttpException(makeError(INTERNAL_ERROR, err), HttpStatus.INTERNAL_SERVER_ERROR);
    }
  }

  @Delete(":database/users/:id")
  async deleteOne(@Param("database") database: string, @Param("id") id: string) {
    const repository = resolveRepository(database);
    if (!repository) {
      throw new HttpException(makeError(NOT_FOUND, `unknown database type: ${database}`), HttpStatus.NOT_FOUND);
    }

    try {
      const deleted = await repository.delete(id);
      if (!deleted) {
        throw new HttpException(makeError(NOT_FOUND, `user with id ${id} not found`), HttpStatus.NOT_FOUND);
      }
      return { success: true };
    } catch (err) {
      if (err instanceof HttpException) throw err;
      throw new HttpException(makeError(INTERNAL_ERROR, err), HttpStatus.INTERNAL_SERVER_ERROR);
    }
  }

  @Delete(":database/users")
  async deleteAll(@Param("database") database: string) {
    const repository = resolveRepository(database);
    if (!repository) {
      throw new HttpException(makeError(NOT_FOUND, `unknown database type: ${database}`), HttpStatus.NOT_FOUND);
    }

    try {
      await repository.deleteAll();
      return { success: true };
    } catch (err) {
      if (err instanceof HttpException) throw err;
      throw new HttpException(makeError(INTERNAL_ERROR, err), HttpStatus.INTERNAL_SERVER_ERROR);
    }
  }

  @Delete(":database/reset")
  async reset(@Param("database") database: string) {
    const repository = resolveRepository(database);
    if (!repository) {
      throw new HttpException(makeError(NOT_FOUND, `unknown database type: ${database}`), HttpStatus.NOT_FOUND);
    }

    try {
      await repository.deleteAll();
      return { status: "ok" };
    } catch (err) {
      if (err instanceof HttpException) throw err;
      throw new HttpException(makeError(INTERNAL_ERROR, err), HttpStatus.INTERNAL_SERVER_ERROR);
    }
  }

  @Get(":database/health")
  async health(@Param("database") database: string) {
    const repository = resolveRepository(database);
    if (!repository) {
      throw new HttpException(makeError(NOT_FOUND, `unknown database type: ${database}`), HttpStatus.NOT_FOUND);
    }

    try {
      const healthy = await repository.healthCheck();
      if (!healthy) {
        throw new HttpException(
          makeError("database unavailable", "health check returned false"),
          HttpStatus.SERVICE_UNAVAILABLE
        );
      }
      return { status: "healthy" };
    } catch (err) {
      if (err instanceof HttpException) throw err;
      throw new HttpException(makeError(INTERNAL_ERROR, err), HttpStatus.INTERNAL_SERVER_ERROR);
    }
  }
}
