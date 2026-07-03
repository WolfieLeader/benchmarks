import { type MiddlewareConsumer, Module, type NestModule, RequestMethod } from "@nestjs/common";
import type { NextFunction, Request, Response } from "express";
import { ParamsController } from "./params.controller";
import { ParamsService } from "./params.service";

// multer (via busboy) defaults a file part with no declared Content-Type to
// "text/plain" per RFC 2046, erasing the distinction the contract draws between
// an explicitly-declared text/plain part and an undeclared one. Capture the raw
// multipart body alongside multer so the declared type can be read from the
// wire. This middleware runs before the route's FileInterceptor, and its `data`
// listener is attached before multer pipes the request, so both see every chunk.
function captureRawBody(req: Request, _res: Response, next: NextFunction) {
  const chunks: Buffer[] = [];
  req.on("data", (chunk: Buffer) => chunks.push(chunk));
  req.on("end", () => {
    (req as Request & { rawBody?: Buffer }).rawBody = Buffer.concat(chunks);
  });
  // Adding a `data` listener switches the stream to flowing mode; pause it so no
  // chunk is emitted (and lost to multer) across Nest's async pipeline before
  // the FileInterceptor attaches. Buffered data replays to both consumers when
  // multer resumes the stream.
  req.pause();
  next();
}

@Module({
  controllers: [ParamsController],
  providers: [ParamsService]
})
export class ParamsModule implements NestModule {
  configure(consumer: MiddlewareConsumer): void {
    consumer.apply(captureRawBody).forRoutes({ path: "params/file", method: RequestMethod.POST });
  }
}
