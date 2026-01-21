import {
  Controller,
  Get,
  Headers,
  HttpCode,
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
import { MAX_FILE_BYTES } from "../consts/defaults";
import { ParamsService } from "./params.service";

@Controller("params")
export class ParamsController {
  constructor(private readonly paramsService: ParamsService) {}

  @Get("/search")
  search(@Query("q") q?: string, @Query("limit") limit?: string) {
    return this.paramsService.parseSearchParams(q, limit);
  }

  @Get("/url/:dynamic")
  urlParams(@Param("dynamic") dynamic: string) {
    return { dynamic };
  }

  @Get("/header")
  headerParams(@Headers("x-custom-header") header?: string) {
    return { header: this.paramsService.parseHeader(header) };
  }

  @Post("/body")
  @HttpCode(200)
  bodyParams(@Req() req: Request) {
    return this.paramsService.validateJsonBody(req.body);
  }

  @Get("/cookie")
  cookieParams(@Req() req: Request, @Res({ passthrough: true }) res: Response) {
    const cookie = this.paramsService.parseCookie(req.cookies);
    res.cookie("bar", "12345", { maxAge: 10_000, httpOnly: true, path: "/" });
    return { cookie };
  }

  @Post("/form")
  @HttpCode(200)
  formParams(@Req() req: Request) {
    this.paramsService.validateFormContentType(req.get("content-type"));
    return this.paramsService.parseFormData(req.body);
  }

  @Post("/file")
  @HttpCode(200)
  @UseInterceptors(
    FileInterceptor("file", {
      storage: multer.memoryStorage(),
      limits: { fileSize: MAX_FILE_BYTES },
    }),
  )
  fileParams(@Req() req: Request, @UploadedFile() file?: Express.Multer.File) {
    this.paramsService.validateMultipartContentType(req.get("content-type"));
    return this.paramsService.processUploadedFile(file);
  }
}
