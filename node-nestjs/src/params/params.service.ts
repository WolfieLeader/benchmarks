import { Injectable, HttpException, HttpStatus } from "@nestjs/common";
import { DEFAULT_LIMIT, MAX_FILE_BYTES, NULL_BYTE, SNIFF_LEN } from "../consts/defaults";
import {
  INVALID_JSON_BODY,
  INVALID_FORM_DATA,
  INVALID_MULTIPART,
  FILE_NOT_FOUND,
  FILE_SIZE_EXCEEDS,
  ONLY_TEXT_PLAIN,
  FILE_NOT_TEXT,
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
    const limitNum = limitStr ? Number(limitStr) : NaN;
    const safeLimit = !limitStr?.includes(".") && Number.isSafeInteger(limitNum) ? limitNum : DEFAULT_LIMIT;

    return { search, limit: safeLimit };
  }

  parseHeader(header?: string): string {
    return header?.trim() || "none";
  }

  validateJsonBody(body: unknown): { body: object } {
    if (typeof body !== "object" || body === null || Array.isArray(body)) {
      throw new HttpException({ error: INVALID_JSON_BODY }, HttpStatus.BAD_REQUEST);
    }
    return { body };
  }

  parseCookie(cookies?: Record<string, string>): string {
    return cookies?.foo?.trim() || "none";
  }

  validateFormContentType(contentType?: string): void {
    const ct = contentType?.toLowerCase() ?? "";
    if (!ct.startsWith("application/x-www-form-urlencoded") && !ct.startsWith("multipart/form-data")) {
      throw new HttpException({ error: INVALID_FORM_DATA }, HttpStatus.BAD_REQUEST);
    }
  }

  parseFormData(body?: { name?: string; age?: string }): FormResult {
    const name = typeof body?.name === "string" ? body.name.trim() || "none" : "none";
    const ageStr = typeof body?.age === "string" ? body.age.trim() : "0";
    const ageNum = Number(ageStr);
    const age = Number.isSafeInteger(ageNum) ? ageNum : 0;

    return { name, age };
  }

  validateMultipartContentType(contentType?: string): void {
    const ct = contentType?.toLowerCase() ?? "";
    if (!ct.startsWith("multipart/form-data")) {
      throw new HttpException({ error: INVALID_MULTIPART }, HttpStatus.BAD_REQUEST);
    }
  }

  processUploadedFile(file?: Express.Multer.File): FileResult {
    if (!file) {
      throw new HttpException({ error: FILE_NOT_FOUND }, HttpStatus.BAD_REQUEST);
    }

    if (!file.mimetype || !file.mimetype.startsWith("text/plain")) {
      throw new HttpException({ error: ONLY_TEXT_PLAIN }, HttpStatus.UNSUPPORTED_MEDIA_TYPE);
    }

    if (file.size > MAX_FILE_BYTES) {
      throw new HttpException({ error: FILE_SIZE_EXCEEDS }, HttpStatus.PAYLOAD_TOO_LARGE);
    }

    const head = file.buffer.subarray(0, SNIFF_LEN);
    if (head.includes(NULL_BYTE)) {
      throw new HttpException({ error: FILE_NOT_TEXT }, HttpStatus.UNSUPPORTED_MEDIA_TYPE);
    }

    if (file.buffer.includes(NULL_BYTE)) {
      throw new HttpException({ error: FILE_NOT_TEXT }, HttpStatus.UNSUPPORTED_MEDIA_TYPE);
    }

    let content: string;
    try {
      const decoder = new TextDecoder("utf-8", { fatal: true });
      content = decoder.decode(file.buffer);
    } catch {
      throw new HttpException({ error: FILE_NOT_TEXT }, HttpStatus.UNSUPPORTED_MEDIA_TYPE);
    }

    return {
      filename: file.originalname,
      size: file.buffer.length,
      content,
    };
  }
}
