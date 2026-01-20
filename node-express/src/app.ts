import express, { type NextFunction, type Request, type Response } from "express";
import cookieParser from "cookie-parser";
import morgan from "morgan";
import { MulterError } from "multer";
import { paramsRouter } from "./routes/params";
import { env } from "./env";

const MAX_REQUEST_BYTES = 10 * 1024 * 1024; // 10 MB

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
    res.json({ message: "Hello, World!" });
  });
  app.get("/health", (_req, res) => {
    res.type("text/plain").send("OK");
  });

  app.use("/params", paramsRouter);

  app.use((_req: Request, res: Response) => {
    res.status(404).json({ error: "not found" });
  });

  app.use((err: unknown, _req: Request, res: Response, _next: NextFunction) => {
    if (err instanceof MulterError && err.code === "LIMIT_FILE_SIZE") {
      res.status(413).json({ error: "file size exceeds limit" });
      return;
    }

    if (err instanceof MulterError) {
      res.status(400).json({ error: "invalid multipart form data" });
      return;
    }

    if (err instanceof SyntaxError && (err as { status?: number }).status === 400 && "body" in err) {
      res.status(400).json({ error: "invalid JSON body" });
      return;
    }

    const message = err instanceof Error ? err.message : "internal error";
    res.status(500).json({ error: message || "internal error" });
  });

  return app;
}
