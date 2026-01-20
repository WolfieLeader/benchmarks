import express, { type NextFunction, type Request, type Response, type Router } from "express";
import multer from "multer";

const MAX_REQUEST_BYTES = 10 * 1024 * 1024; // 10 MB
const MAX_FILE_BYTES = 1 << 20; // 1MB
const SNIFF_LEN = 512;
const NULL_BYTE = 0x00;

const upload = multer({
  storage: multer.memoryStorage(),
  limits: { fileSize: MAX_FILE_BYTES },
});

const formParser = multer({
  limits: { fieldSize: MAX_REQUEST_BYTES },
}).none();

export const paramsRouter: Router = express.Router();

paramsRouter.get("/search", (req: Request, res: Response) => {
  const qValue = Array.isArray(req.query.q) ? req.query.q[0] : req.query.q;
  const q = typeof qValue === "string" && qValue.trim() !== "" ? qValue.trim() : "none";

  const limitValue = Array.isArray(req.query.limit) ? req.query.limit[0] : req.query.limit;
  const limitStr = typeof limitValue === "string" ? limitValue : undefined;
  const limitNum = Number(limitStr);
  const limit = limitStr !== undefined && !limitStr.includes(".") && Number.isSafeInteger(limitNum) ? limitNum : 10;

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
    res.status(400).json({ error: "invalid JSON body" });
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
    res.status(400).json({ error: "invalid form data" });
    return;
  }

  const name = typeof body.name === "string" ? body.name.trim() || "none" : "none";

  const ageStr = typeof body.age === "string" ? body.age.trim() : "0";
  const ageNum = Number(ageStr);
  const age = !ageStr.includes(".") && Number.isSafeInteger(ageNum) ? ageNum : 0;

  res.json({ name, age });
}

paramsRouter.post("/form", (req: Request, res: Response) => {
  const contentType = req.get("content-type")?.toLowerCase() ?? "";
  if (!contentType.startsWith("application/x-www-form-urlencoded") && !contentType.startsWith("multipart/form-data")) {
    res.status(400).json({ error: "invalid form data" });
    return;
  }

  if (req.is("multipart/form-data")) {
    formParser(req, res, (err?: unknown) => {
      if (err) {
        res.status(400).json({ error: "invalid form data" });
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
    res.status(400).json({ error: "invalid multipart form data" });
    return;
  }

  upload.single("file")(req, res, (err?: unknown) => {
    if (err) {
      next(err);
      return;
    }

    const file = req.file;
    if (!file) {
      res.status(400).json({ error: "file not found in form data" });
      return;
    }

    if (!file.mimetype || !file.mimetype.startsWith("text/plain")) {
      res.status(415).json({ error: "only text/plain files are allowed" });
      return;
    }

    if (file.size > MAX_FILE_BYTES) {
      res.status(413).json({ error: "file size exceeds limit" });
      return;
    }

    const head = file.buffer.subarray(0, SNIFF_LEN);
    if (head.includes(NULL_BYTE)) {
      res.status(415).json({ error: "file does not look like plain text" });
      return;
    }

    if (file.buffer.includes(NULL_BYTE)) {
      res.status(415).json({ error: "file does not look like plain text" });
      return;
    }

    let content: string;
    try {
      const decoder = new TextDecoder("utf-8", { fatal: true });
      content = decoder.decode(file.buffer);
    } catch {
      res.status(415).json({ error: "file does not look like plain text" });
      return;
    }

    res.json({
      filename: file.originalname,
      size: file.buffer.length,
      content,
    });
  });
});
