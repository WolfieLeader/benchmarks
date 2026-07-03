import "reflect-metadata";
import {
  type ArgumentsHost,
  BadRequestException,
  Catch,
  type ExceptionFilter,
  HttpException,
  NotFoundException,
  PayloadTooLargeException
} from "@nestjs/common";
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

// A BadRequestException whose response still carries Nest's default
// `error: "Bad Request"` — i.e. framework-generated, not an app-level 400.
function isDefaultBadRequest(exception: HttpException): boolean {
  const res = exception.getResponse();
  return typeof res === "object" && res !== null && (res as { error?: unknown }).error === "Bad Request";
}

// The parse-error detail carried on the exception's response `message`.
function messageOf(exception: HttpException): string | undefined {
  const res = exception.getResponse();
  if (typeof res === "object" && res !== null && typeof (res as { message?: unknown }).message === "string") {
    return (res as { message: string }).message;
  }
  return exception.message || undefined;
}

@Catch()
class GlobalExceptionFilter implements ExceptionFilter {
  catch(exception: unknown, host: ArgumentsHost) {
    const ctx = host.switchToHttp();
    const response = ctx.getResponse();

    if (exception instanceof NotFoundException) {
      response.status(404).json({ error: NOT_FOUND });
      return;
    }

    // multer's LIMIT_FILE_SIZE is transformed into a PayloadTooLargeException by
    // @nestjs/platform-express's FileInterceptor before it reaches this filter.
    if (exception instanceof PayloadTooLargeException) {
      response.status(413).json(makeError(FILE_SIZE_EXCEEDS, messageOf(exception)));
      return;
    }

    // Body/JSON parse failures surface as a framework-default BadRequestException
    // (getResponse().error === "Bad Request"). Every app-level 400 is thrown with
    // a specific error string via makeError, so a still-default "Bad Request"
    // means the parser rejected the payload.
    if (exception instanceof BadRequestException && isDefaultBadRequest(exception)) {
      response.status(400).json(makeError(INVALID_JSON_BODY, messageOf(exception)));
      return;
    }

    // Fallback: a raw body-parser SyntaxError (if it ever escapes Nest's parser).
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
        let errorMsg: string | undefined;
        if (typeof payload.error === "string") errorMsg = payload.error;
        else if (typeof payload.message === "string") errorMsg = payload.message;
        if (errorMsg) {
          response.status(status).json({ error: errorMsg, ...(payload.details && { details: payload.details }) });
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
