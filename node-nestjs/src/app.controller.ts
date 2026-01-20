import { Controller, Get, Header } from "@nestjs/common";

@Controller()
export class AppController {
  @Get()
  hello() {
    return { message: "Hello, World!" };
  }

  @Get("health")
  @Header("Content-Type", "text/plain")
  health() {
    return "OK";
  }
}
