import { Module } from "@nestjs/common";

import { AppController } from "./app.controller";
import { DbModule } from "./db/db.module";
import { ParamsModule } from "./params/params.module";

@Module({
  imports: [ParamsModule, DbModule],
  controllers: [AppController]
})
export class AppModule {}
