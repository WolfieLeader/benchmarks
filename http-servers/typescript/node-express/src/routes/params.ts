import express, { type NextFunction, type Request, type Response, type Router } from "express";
import multer from "multer";
import { MAX_FILE_BYTES, MAX_REQUEST_BYTES, NULL_BYTE, SNIFF_LEN, DEFAULT_LIMIT } from "../consts/defaults";
import {
  INVALID_JSON_BODY,
  INVALID_FORM_DATA,
  INVALID_MULTIPART,
  FILE_NOT_FOUND,
  FILE_SIZE_EXCEEDS,
  ONLY_TEXT_PLAIN,
  FILE_NOT_TEXT,
  makeError
} from "../consts/errors";

const upload = multer({
  storage: multer.memoryStorage(),
  limits: { fileSize: MAX_FILE_BYTES }
});

const formParser = multer({
  limits: { fieldSize: MAX_REQUEST_BYTES }
}).none();

export const paramsRouter: Router = express.Router();

paramsRouter.get("/search", (req: Request, res: Response) => {
  const qValue = Array.isArray(req.query.q) ? req.query.q[0] : req.query.q;
  const q = typeof qValue === "string" && qValue.trim() !== "" ? qValue.trim() : "none";

  const limitValue = Array.isArray(req.query.limit) ? req.query.limit[0] : req.query.limit;
  const limitStr = typeof limitValue === "string" ? limitValue : undefined;
  const limitNum = Number(limitStr);
  const limit =
    limitStr !== undefined && !limitStr.includes(".") && Number.isSafeInteger(limitNum) ? limitNum : DEFAULT_LIMIT;

  res.json({ search: q, limit });
});

paramsRouter.get("/url/:dynamic", (req: Request, res: Response) => {
  res.json({ dynamic: req.params.dynamic });
});

paramsRouter.get("/header", (req: Request, res: Response) => {
  const header = req.get("X-Custom-Header")?.trim() || "none";
  res.json({ header });
});

paramsRouter.post("/body", (req: Request, res: Response) => {
  const body = req.body;

  if (typeof body !== "object" || body === null || Array.isArray(body)) {
    res.status(400).json(makeError(INVALID_JSON_BODY, "expected a JSON object"));
    return;
  }

  res.json({ body });
});

paramsRouter.get("/cookie", (req: Request, res: Response) => {
  const cookie = req.cookies?.foo?.trim() || "none";
  res.cookie("bar", "12345", { maxAge: 10_000, httpOnly: true, path: "/" });
  res.json({ cookie });
});

function handleForm(req: Request, res: Response) {
  const body = req.body;
  if (typeof body !== "object" || body === null || Array.isArray(body)) {
    res.status(400).json(makeError(INVALID_FORM_DATA, "expected form fields"));
    return;
  }

  const name = typeof body.name === "string" ? body.name.trim() || "none" : "none";

  const ageStr = typeof body.age === "string" ? body.age.trim() : "0";
  const ageNum = Number(ageStr);
  const age = Number.isSafeInteger(ageNum) ? ageNum : 0;

  res.json({ name, age });
}

paramsRouter.post("/form", (req: Request, res: Response) => {
  const contentType = req.get("content-type")?.toLowerCase() ?? "";
  if (!contentType.startsWith("application/x-www-form-urlencoded") && !contentType.startsWith("multipart/form-data")) {
    res
      .status(400)
      .json(
        makeError(INVALID_FORM_DATA, "expected content-type: application/x-www-form-urlencoded or multipart/form-data")
      );
    return;
  }

  if (req.is("multipart/form-data")) {
    formParser(req, res, (err?: unknown) => {
      if (err) {
        res.status(400).json(makeError(INVALID_FORM_DATA, err));
        return;
      }
      handleForm(req, res);
    });
    return;
  }

  handleForm(req, res);
});

paramsRouter.post("/file", (req: Request, res: Response, next: NextFunction) => {
  const contentType = req.get("content-type")?.toLowerCase() ?? "";
  if (!contentType.startsWith("multipart/form-data")) {
    res.status(400).json(makeError(INVALID_MULTIPART, "expected content-type: multipart/form-data"));
    return;
  }

  upload.single("file")(req, res, (err?: unknown) => {
    if (err) {
      next(err);
      return;
    }

    const file = req.file;
    if (!file) {
      res.status(400).json(makeError(FILE_NOT_FOUND, "no file field named 'file' in form data"));
      return;
    }

    if (!file.mimetype || !file.mimetype.startsWith("text/plain")) {
      res.status(415).json(makeError(ONLY_TEXT_PLAIN, `received mimetype: ${file.mimetype || "unknown"}`));
      return;
    }

    if (file.size > MAX_FILE_BYTES) {
      res.status(413).json(makeError(FILE_SIZE_EXCEEDS, `file size ${file.size} exceeds limit ${MAX_FILE_BYTES}`));
      return;
    }

    const head = file.buffer.subarray(0, SNIFF_LEN);
    if (head.includes(NULL_BYTE)) {
      res.status(415).json(makeError(FILE_NOT_TEXT, "file contains null bytes in header"));
      return;
    }

    if (file.buffer.includes(NULL_BYTE)) {
      res.status(415).json(makeError(FILE_NOT_TEXT, "file contains null bytes"));
      return;
    }

    let content: string;
    try {
      const decoder = new TextDecoder("utf-8", { fatal: true });
      content = decoder.decode(file.buffer);
    } catch {
      res.status(415).json(makeError(FILE_NOT_TEXT, "file is not valid UTF-8"));
      return;
    }

    res.json({
      filename: file.originalname,
      size: file.buffer.length,
      content
    });
  });
});
