import { Controller, Get, Header } from "@nestjs/common";
import { getAllDatabaseStatuses } from "./db/database/repository";

@Controller()
export class AppController {
  @Get()
  @Header("Content-Type", "text/plain")
  hello() {
    return "OK";
  }

  @Get("health")
  async health() {
    const databases = await getAllDatabaseStatuses();
    return { status: "healthy", databases };
  }
}
