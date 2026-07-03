import {
  DEFAULT_LIMIT,
  EXPECTED_FORM_CONTENT_TYPE,
  EXPECTED_MULTIPART_CONTENT_TYPE,
  FILE_NOT_FOUND,
  FILE_NOT_TEXT,
  FILE_SIZE_EXCEEDS,
  INVALID_FORM_DATA,
  INVALID_JSON_BODY,
  INVALID_MULTIPART,
  makeError,
  MAX_FILE_BYTES,
  MAX_REQUEST_BYTES,
  NULL_BYTE,
  ONLY_TEXT_PLAIN,
  SNIFF_LEN
} from "@bench/shared";
import express, { type NextFunction, type Request, type Response, type Router } from "express";
import multer from "multer";

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
  const limit = Number.isSafeInteger(limitNum) && !limitStr?.includes(".") ? limitNum : DEFAULT_LIMIT;

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
  const { body } = req;
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
  const { body } = req;
  if (typeof body !== "object" || body === null || Array.isArray(body)) {
    res.status(400).json(makeError(INVALID_FORM_DATA, "expected form fields"));
    return;
  }

  const name = typeof body.name === "string" ? body.name.trim() || "none" : "none";

  const ageStr = typeof body.age === "string" ? body.age.trim() || "0" : "0";
  const ageNum = Number(ageStr);
  const age = Number.isSafeInteger(ageNum) ? ageNum : 0;

  res.json({ name, age });
}

paramsRouter.post("/form", (req: Request, res: Response) => {
  const contentType = req.get("content-type")?.toLowerCase() ?? "";
  if (!contentType.startsWith("application/x-www-form-urlencoded") && !contentType.startsWith("multipart/form-data")) {
    res.status(400).json(makeError(INVALID_FORM_DATA, EXPECTED_FORM_CONTENT_TYPE));
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

// Multer (via busboy) defaults a file part with no declared Content-Type to
// "text/plain" per RFC 2046, which erases the distinction the contract draws
// between an explicitly-declared text/plain part and an undeclared one. Capture
// the raw multipart body alongside multer so the file part's declared type can
// be read back from the wire — the tee attaches its `data` listener before
// multer pipes the request, so both consumers see every chunk.
// Retention is capped: the declared-type headers sit in the first bytes of the
// part, and multer 413s anything past MaxFileBytes anyway — without the cap a
// malicious oversized upload would be buffered in full alongside multer's copy.
const maxRawBodyBytes = 2 * 1024 * 1024;

function captureRawBody(req: Request, _res: Response, next: NextFunction) {
  const chunks: Buffer[] = [];
  let captured = 0;
  req.on("data", (chunk: Buffer) => {
    if (captured >= maxRawBodyBytes) return;
    captured += chunk.length;
    chunks.push(chunk);
  });
  req.on("end", () => {
    (req as RawBodyRequest).rawBody = Buffer.concat(chunks);
  });
  next();
}

type RawBodyRequest = Request & { rawBody?: Buffer };

// The declared Content-Type of the file part (the part with a filename), or
// null when the part carried no Content-Type header at all.
function declaredFileContentType(rawBody: Buffer | undefined, requestContentType: string): string | null {
  if (!rawBody) return null;
  const boundaryMatch = /boundary=(?:"([^"]+)"|([^;]+))/i.exec(requestContentType);
  const boundary = (boundaryMatch?.[1] ?? boundaryMatch?.[2])?.trim();
  if (!boundary) return null;

  const raw = rawBody.toString("latin1");
  for (const segment of raw.split(`--${boundary}`)) {
    const headerEnd = segment.indexOf("\r\n\r\n");
    if (headerEnd === -1) continue;
    const headers = segment.slice(0, headerEnd);
    if (!/content-disposition:[^\r\n]*\bfilename=/i.test(headers)) continue;
    const typeMatch = /\r\ncontent-type:\s*([^\r\n]+)/i.exec(`\r\n${headers}`);
    return typeMatch?.[1]?.trim() ?? null;
  }
  return null;
}

function looksLikeText(bytes: Buffer): boolean {
  if (bytes.includes(NULL_BYTE)) return false;
  try {
    new TextDecoder("utf-8", { fatal: true }).decode(bytes);
    return true;
  } catch {
    return false;
  }
}

paramsRouter.post("/file", (req: Request, res: Response, next: NextFunction) => {
  const contentType = req.get("content-type")?.toLowerCase() ?? "";
  if (!contentType.startsWith("multipart/form-data")) {
    res.status(400).json(makeError(INVALID_MULTIPART, EXPECTED_MULTIPART_CONTENT_TYPE));
    return;
  }

  captureRawBody(req, res, () => {
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

      const head = file.buffer.subarray(0, SNIFF_LEN);
      const declared = declaredFileContentType((req as RawBodyRequest).rawBody, req.get("content-type") ?? "");
      if (declared !== null) {
        // Trust the declared type for the allow-decision; a lie is caught by the
        // content inspection below (-> FILE_NOT_TEXT).
        if (!declared.toLowerCase().startsWith("text/plain")) {
          res.status(415).json(makeError(ONLY_TEXT_PLAIN, `received mimetype: ${declared}`));
          return;
        }
      } else if (!looksLikeText(head)) {
        // No declared type: sniff the bytes; non-text is rejected as a type error.
        res.status(415).json(makeError(ONLY_TEXT_PLAIN, "received mimetype: unknown"));
        return;
      }

      if (file.size > MAX_FILE_BYTES) {
        res.status(413).json(makeError(FILE_SIZE_EXCEEDS, `file size ${file.size} exceeds limit ${MAX_FILE_BYTES}`));
        return;
      }

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
        content = new TextDecoder("utf-8", { fatal: true }).decode(file.buffer);
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
});
