import { HttpException, HttpStatus, Injectable } from "@nestjs/common";
import { DEFAULT_LIMIT, MAX_FILE_BYTES, NULL_BYTE, SNIFF_LEN } from "../consts/defaults";
import {
  FILE_NOT_FOUND,
  FILE_NOT_TEXT,
  FILE_SIZE_EXCEEDS,
  INVALID_FORM_DATA,
  INVALID_JSON_BODY,
  INVALID_MULTIPART,
  makeError,
  ONLY_TEXT_PLAIN
} from "../consts/errors";

export interface SearchResult {
  search: string;
  limit: number;
}

export interface FormResult {
  name: string;
  age: number;
}

export interface FileResult {
  filename: string;
  size: number;
  content: string;
}

@Injectable()
export class ParamsService {
  parseSearchParams(q?: string, limit?: string): SearchResult {
    const search = q?.trim() || "none";
    const limitStr = limit?.trim();
    const limitNum = limitStr ? Number(limitStr) : Number.NaN;
    const safeLimit = Number.isSafeInteger(limitNum) ? limitNum : DEFAULT_LIMIT;

    return { search, limit: safeLimit };
  }

  parseHeader(header?: string): string {
    return header?.trim() || "none";
  }

  validateJsonBody(body: unknown): { body: object } {
    if (typeof body !== "object" || body === null || Array.isArray(body)) {
      throw new HttpException(makeError(INVALID_JSON_BODY, "expected a JSON object"), HttpStatus.BAD_REQUEST);
    }
    return { body };
  }

  parseCookie(cookies?: Record<string, string>): string {
    return cookies?.foo?.trim() || "none";
  }

  validateFormContentType(contentType?: string): void {
    const ct = contentType?.toLowerCase() ?? "";
    if (!ct.startsWith("application/x-www-form-urlencoded") && !ct.startsWith("multipart/form-data")) {
      throw new HttpException(
        makeError(INVALID_FORM_DATA, "expected content-type: application/x-www-form-urlencoded or multipart/form-data"),
        HttpStatus.BAD_REQUEST
      );
    }
  }

  parseFormData(body?: { name?: string; age?: string }): FormResult {
    const name = typeof body?.name === "string" ? body.name.trim() || "none" : "none";
    const ageStr = typeof body?.age === "string" && body.age.trim() !== "" ? body.age.trim() : "0";
    const ageNum = Number(ageStr);
    const age = Number.isSafeInteger(ageNum) ? ageNum : 0;

    return { name, age };
  }

  validateMultipartContentType(contentType?: string): void {
    const ct = contentType?.toLowerCase() ?? "";
    if (!ct.startsWith("multipart/form-data")) {
      throw new HttpException(
        makeError(INVALID_MULTIPART, "expected content-type: multipart/form-data"),
        HttpStatus.BAD_REQUEST
      );
    }
  }

  processUploadedFile(file?: Express.Multer.File): FileResult {
    if (!file) {
      throw new HttpException(
        makeError(FILE_NOT_FOUND, "no file field named 'file' in form data"),
        HttpStatus.BAD_REQUEST
      );
    }

    if (!file.mimetype || !file.mimetype.startsWith("text/plain")) {
      throw new HttpException(
        makeError(ONLY_TEXT_PLAIN, `received mimetype: ${file.mimetype || "unknown"}`),
        HttpStatus.UNSUPPORTED_MEDIA_TYPE
      );
    }

    if (file.size > MAX_FILE_BYTES) {
      throw new HttpException(
        makeError(FILE_SIZE_EXCEEDS, `file size ${file.size} exceeds limit ${MAX_FILE_BYTES}`),
        HttpStatus.PAYLOAD_TOO_LARGE
      );
    }

    const head = file.buffer.subarray(0, SNIFF_LEN);
    if (head.includes(NULL_BYTE)) {
      throw new HttpException(
        makeError(FILE_NOT_TEXT, "file contains null bytes in header"),
        HttpStatus.UNSUPPORTED_MEDIA_TYPE
      );
    }

    if (file.buffer.includes(NULL_BYTE)) {
      throw new HttpException(makeError(FILE_NOT_TEXT, "file contains null bytes"), HttpStatus.UNSUPPORTED_MEDIA_TYPE);
    }

    let content: string;
    try {
      const decoder = new TextDecoder("utf-8", { fatal: true });
      content = decoder.decode(file.buffer);
    } catch {
      throw new HttpException(makeError(FILE_NOT_TEXT, "file is not valid UTF-8"), HttpStatus.UNSUPPORTED_MEDIA_TYPE);
    }

    return {
      filename: file.originalname,
      size: file.buffer.length,
      content
    };
  }
}
