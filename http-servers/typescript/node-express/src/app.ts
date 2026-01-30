import express, { type NextFunction, type Request, type Response } from "express";
import cookieParser from "cookie-parser";
import morgan from "morgan";
import { MulterError } from "multer";
import { dbRouter } from "./routes/db";
import { paramsRouter } from "./routes/params";
import { env } from "./config/env";
import {
  NOT_FOUND,
  FILE_SIZE_EXCEEDS,
  INVALID_MULTIPART,
  INVALID_JSON_BODY,
  INTERNAL_ERROR,
  makeError
} from "./consts/errors";
import { MAX_REQUEST_BYTES } from "./consts/defaults";

export function createApp(): express.Express {
  const app = express();

  app.disable("x-powered-by");

  if (env.ENV !== "prod") {
    app.use(morgan("dev"));
  }

  app.use(express.json({ limit: MAX_REQUEST_BYTES }));
  app.use(express.urlencoded({ extended: false, limit: MAX_REQUEST_BYTES }));
  app.use(cookieParser());

  app.get("/", (_req, res) => {
    res.json({ message: "Hello World" });
  });
  app.get("/health", (_req, res) => {
    res.json({ status: "healthy" });
  });

  app.use("/params", paramsRouter);
  app.use("/db", dbRouter);

  app.use((_req: Request, res: Response) => {
    res.status(404).json({ error: NOT_FOUND });
  });

  app.use((err: unknown, _req: Request, res: Response, _next: NextFunction) => {
    if (err instanceof MulterError && err.code === "LIMIT_FILE_SIZE") {
      res.status(413).json(makeError(FILE_SIZE_EXCEEDS, err.message));
      return;
    }

    if (err instanceof MulterError) {
      res.status(400).json(makeError(INVALID_MULTIPART, err.message));
      return;
    }

    if (err instanceof SyntaxError && (err as { status?: number }).status === 400 && "body" in err) {
      res.status(400).json(makeError(INVALID_JSON_BODY, err.message));
      return;
    }

    res.status(500).json(makeError(INTERNAL_ERROR, err));
  });

  return app;
}
