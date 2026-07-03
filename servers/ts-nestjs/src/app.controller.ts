import { Controller, Get, Header } from "@nestjs/common";

@Controller()
export class AppController {
  @Get()
  hello() {
    return { hello: "world" };
  }

  @Get("health")
  @Header("Content-Type", "text/plain")
  health() {
    return "OK";
  }
}
