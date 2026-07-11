import {
  env,
  INVALID_N,
  INVALID_TOKEN,
  makeError,
  parseComputeRounds,
  sha256Chain,
  signToken,
  VALIDATION_FAILED,
  validateWebPayload,
  verifyToken,
  type WebTokenClaims
} from "@bench/shared";
import {
  Body,
  Controller,
  Get,
  Header,
  Headers,
  HttpCode,
  HttpException,
  HttpStatus,
  Post,
  Query
} from "@nestjs/common";

// The /html canon: greeting + fruit list + labeled total, rendered as a
// server-side template literal. The contract matches each value with
// htmlContains (whitespace-tolerant), not a byte-exact body.
const FRUITS = ["apple", "banana", "cherry"];

function renderHtml(): string {
  const items = FRUITS.map((fruit) => `    <li>${fruit}</li>`).join("\n");
  return `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>Benchmark</title></head>
<body>
  <h1>Hello, Alice</h1>
  <ul>
${items}
  </ul>
  <p>Total: 42</p>
</body>
</html>`;
}

// The web suite's logic lives entirely in @bench/shared (validate rules, the
// SHA-256 chain, the jose JWT helpers); this controller is pure HTTP glue —
// header extraction, status codes, and the canonical error shape via the global
// exception filter — so it needs no dedicated service (unlike ParamsService).
@Controller()
export class WebController {
  @Get("html")
  @Header("Content-Type", "text/html")
  html(): string {
    return renderHtml();
  }

  @Get("jwt/sign")
  async jwtSign(): Promise<{ token: string }> {
    const token = await signToken(env.JWT_SECRET);
    return { token };
  }

  @Get("jwt/verify")
  async jwtVerify(@Headers("authorization") authorization?: string): Promise<WebTokenClaims> {
    const token = authorization?.startsWith("Bearer ") ? authorization.slice(7).trim() : "";
    if (!token) {
      throw new HttpException(makeError(INVALID_TOKEN, "missing bearer token"), HttpStatus.UNAUTHORIZED);
    }
    try {
      return await verifyToken(env.JWT_SECRET, token);
    } catch (err) {
      throw new HttpException(makeError(INVALID_TOKEN, err), HttpStatus.UNAUTHORIZED);
    }
  }

  @Post("validate")
  @HttpCode(200)
  validate(@Body() body: unknown): { valid: true } {
    const result = validateWebPayload(body);
    if (!result.ok) {
      throw new HttpException(makeError(VALIDATION_FAILED, result.details), HttpStatus.BAD_REQUEST);
    }
    return { valid: true };
  }

  @Get("compute")
  compute(@Query("n") n?: string): { result: string } {
    const rounds = parseComputeRounds(n);
    if (rounds === null) {
      throw new HttpException(makeError(INVALID_N, "n must be an integer >= 1"), HttpStatus.BAD_REQUEST);
    }
    return { result: sha256Chain(rounds) };
  }
}
