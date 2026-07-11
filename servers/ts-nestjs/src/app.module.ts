import { Module } from "@nestjs/common";

import { AppController } from "./app.controller";
import { DbModule } from "./db/db.module";
import { ParamsModule } from "./params/params.module";
import { WebModule } from "./web/web.module";

@Module({
  imports: [ParamsModule, DbModule, WebModule],
  controllers: [AppController]
})
export class AppModule {}
