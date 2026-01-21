import {
  Controller,
  Get,
  Headers,
  HttpException,
  HttpStatus,
  Param,
  Post,
  Query,
  Req,
  Res,
  UploadedFile,
  UseInterceptors,
} from "@nestjs/common";
import { FileInterceptor } from "@nestjs/platform-express";
import type { Request, Response } from "express";
import multer from "multer";
import { MAX_FILE_BYTES, NULL_BYTE, SNIFF_LEN, DEFAULT_LIMIT } from "../consts/defaults";
import {
  INVALID_JSON_BODY,
  INVALID_FORM_DATA,
  INVALID_MULTIPART,
  FILE_NOT_FOUND,
  FILE_SIZE_EXCEEDS,
  INVALID_FILE_TYPE,
  NOT_PLAIN_TEXT,
} from "../consts/errors";

@Controller("params")
export class ParamsController {
  @Get("/search")
  search(@Query("q") q?: string, @Query("limit") limit?: string) {
    const search = q?.trim() || "none";

    const limitStr = limit ?? "";
    const limitNum = Number(limitStr);
    const safeLimit = !limitStr.includes(".") && Number.isSafeInteger(limitNum) ? limitNum : DEFAULT_LIMIT;

    return { search, limit: safeLimit };
  }

  @Get("/url/:dynamic")
  urlParams(@Param("dynamic") dynamic: string) {
    return { dynamic };
  }

  @Get("/header")
  headerParams(@Headers("x-custom-header") header?: string) {
    return { header: header?.trim() || "none" };
  }

  @Post("/body")
  bodyParams(@Req() req: Request) {
    const body = req.body;

    if (typeof body !== "object" || body === null || Array.isArray(body)) {
      throw new HttpException({ error: INVALID_JSON_BODY }, HttpStatus.BAD_REQUEST);
    }

    return { body };
  }

  @Get("/cookie")
  cookieParams(@Req() req: Request, @Res({ passthrough: true }) res: Response) {
    const cookie = req.cookies?.foo?.trim() || "none";
    res.cookie("bar", "12345", { maxAge: 10_000, httpOnly: true, path: "/" });
    return { cookie };
  }

  @Post("/form")
  formParams(@Req() req: Request) {
    const contentType = req.get("content-type")?.toLowerCase() ?? "";
    if (
      !contentType.startsWith("application/x-www-form-urlencoded") &&
      !contentType.startsWith("multipart/form-data")
    ) {
      throw new HttpException({ error: INVALID_FORM_DATA }, HttpStatus.BAD_REQUEST);
    }

    const name = typeof req.body?.name === "string" ? req.body.name.trim() || "none" : "none";

    const ageStr = typeof req.body?.age === "string" ? req.body.age.trim() : "0";
    const ageNum = Number(ageStr);
    const age = Number.isSafeInteger(ageNum) ? ageNum : 0;

    return { name, age };
  }

  @Post("/file")
  @UseInterceptors(
    FileInterceptor("file", {
      storage: multer.memoryStorage(),
      limits: { fileSize: MAX_FILE_BYTES },
    }),
  )
  fileParams(@Req() req: Request, @UploadedFile() file?: Express.Multer.File) {
    const contentType = req.get("content-type")?.toLowerCase() ?? "";
    if (!contentType.startsWith("multipart/form-data")) {
      throw new HttpException({ error: INVALID_MULTIPART }, HttpStatus.BAD_REQUEST);
    }

    if (!file) {
      throw new HttpException({ error: FILE_NOT_FOUND }, HttpStatus.BAD_REQUEST);
    }

    if (!file.mimetype || !file.mimetype.startsWith("text/plain")) {
      throw new HttpException({ error: INVALID_FILE_TYPE }, HttpStatus.UNSUPPORTED_MEDIA_TYPE);
    }

    if (file.size > MAX_FILE_BYTES) {
      throw new HttpException({ error: FILE_SIZE_EXCEEDS }, HttpStatus.PAYLOAD_TOO_LARGE);
    }

    const head = file.buffer.subarray(0, SNIFF_LEN);
    if (head.includes(NULL_BYTE)) {
      throw new HttpException({ error: NOT_PLAIN_TEXT }, HttpStatus.UNSUPPORTED_MEDIA_TYPE);
    }

    if (file.buffer.includes(NULL_BYTE)) {
      throw new HttpException({ error: NOT_PLAIN_TEXT }, HttpStatus.UNSUPPORTED_MEDIA_TYPE);
    }

    let content: string;
    try {
      const decoder = new TextDecoder("utf-8", { fatal: true });
      content = decoder.decode(file.buffer);
    } catch {
      throw new HttpException({ error: NOT_PLAIN_TEXT }, HttpStatus.UNSUPPORTED_MEDIA_TYPE);
    }

    return {
      filename: file.originalname,
      size: file.buffer.length,
      content,
    };
  }
}
