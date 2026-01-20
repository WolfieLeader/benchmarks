import "reflect-metadata";
import cookieParser from "cookie-parser";
import express from "express";
import morgan from "morgan";
import { NestFactory } from "@nestjs/core";
import { ValidationPipe, NotFoundException, HttpException } from "@nestjs/common";

import { AppModule } from "./app.module";
import { env } from "./env";

async function bootstrap() {
  const app = await NestFactory.create(AppModule);

  if (env.ENV !== "prod") {
    app.use(morgan("dev"));
  }

  app.use(express.json({ limit: "1mb" }));
  app.use(express.urlencoded({ extended: false }));
  app.use(cookieParser());

  app.useGlobalPipes(
    new ValidationPipe({
      whitelist: false,
      transform: true,
    }),
  );

  app.useGlobalFilters({
    catch(exception: unknown, host: any) {
      const ctx = host.switchToHttp();
      const response = ctx.getResponse();

      if (exception instanceof NotFoundException) {
        response.status(404).json({ error: "not found" });
        return;
      }

      if (exception instanceof HttpException) {
        const status = exception.getStatus();
        const exceptionResponse = exception.getResponse();
        if (typeof exceptionResponse === "object" && exceptionResponse !== null) {
          response.status(status).json(exceptionResponse);
        } else {
          response.status(status).json({ error: exceptionResponse });
        }
        return;
      }

      const message = exception instanceof Error ? exception.message : "internal error";
      response.status(500).json({ error: message || "internal error" });
    },
  });

  await app.listen(env.PORT, env.HOST);

  console.log(`Server running at http://${env.HOST}:${env.PORT}/`);
}

bootstrap();
