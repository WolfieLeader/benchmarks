import { Controller, Get, Header } from "@nestjs/common";

@Controller()
export class AppController {
  @Get()
  @Header("Content-Type", "text/plain")
  hello() {
    return "OK";
  }

  @Get("health")
  health() {
    return { message: "Hello World" };
  }
}
