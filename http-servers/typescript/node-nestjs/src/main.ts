import "reflect-metadata";
import { type ArgumentsHost, Catch, type ExceptionFilter, HttpException, NotFoundException } from "@nestjs/common";
import { NestFactory } from "@nestjs/core";
import cookieParser from "cookie-parser";
import express from "express";
import morgan from "morgan";
import multer from "multer";

import { AppModule } from "./app.module";
import { env } from "./config/env";
import { MAX_REQUEST_BYTES } from "./consts/defaults";
import {
  FILE_SIZE_EXCEEDS,
  INTERNAL_ERROR,
  INVALID_JSON_BODY,
  INVALID_MULTIPART,
  makeError,
  NOT_FOUND
} from "./consts/errors";
import { disconnectDatabases, initializeDatabases } from "./db/database/repository";

@Catch()
class GlobalExceptionFilter implements ExceptionFilter {
  catch(exception: unknown, host: ArgumentsHost) {
    const ctx = host.switchToHttp();
    const response = ctx.getResponse();

    if (exception instanceof NotFoundException) {
      response.status(404).json({ error: NOT_FOUND });
      return;
    }

    if (exception instanceof SyntaxError && (exception as { type?: string }).type === "entity.parse.failed") {
      response.status(400).json(makeError(INVALID_JSON_BODY, exception.message));
      return;
    }

    if (exception instanceof multer.MulterError) {
      if (exception.code === "LIMIT_FILE_SIZE") {
        response.status(413).json(makeError(FILE_SIZE_EXCEEDS, exception.message));
        return;
      }
      response.status(400).json(makeError(INVALID_MULTIPART, exception.message));
      return;
    }

    if (exception instanceof HttpException) {
      const status = exception.getStatus();
      const exceptionResponse = exception.getResponse();
      if (typeof exceptionResponse === "object" && exceptionResponse !== null) {
        const payload = exceptionResponse as { error?: string; message?: string | string[]; details?: string };
        if (typeof payload.error === "string") {
          response.status(status).json({ error: payload.error, ...(payload.details && { details: payload.details }) });
          return;
        }
        if (typeof payload.message === "string") {
          response
            .status(status)
            .json({ error: payload.message, ...(payload.details && { details: payload.details }) });
          return;
        }
      }
      response.status(status).json({ error: exceptionResponse });
      return;
    }

    response.status(500).json(makeError(INTERNAL_ERROR, exception));
  }
}

async function bootstrap() {
  await initializeDatabases();

  const app = await NestFactory.create(AppModule);

  if (env.ENV !== "prod") {
    app.use(morgan("dev"));
  }

  app.use(express.json({ limit: MAX_REQUEST_BYTES }));
  app.use(express.urlencoded({ extended: false, limit: MAX_REQUEST_BYTES }));
  app.use(cookieParser());

  app.useGlobalFilters(new GlobalExceptionFilter());

  await app.listen(env.PORT, env.HOST);

  console.log(`Server running at http://${env.HOST}:${env.PORT}/`);

  async function shutdown() {
    console.log("Shutting down...");
    await app.close();
    await disconnectDatabases();
    process.exit(0);
  }

  process.on("SIGTERM", shutdown);
  process.on("SIGINT", shutdown);
}

bootstrap();
