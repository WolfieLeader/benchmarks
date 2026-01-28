import { Module } from "@nestjs/common";
import { DbController } from "./db.controller";

@Module({
  controllers: [DbController]
})
export class DbModule {}
