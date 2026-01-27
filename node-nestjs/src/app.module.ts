import { Module } from "@nestjs/common";

import { AppController } from "./app.controller";
import { ParamsModule } from "./params/params.module";

@Module({
  imports: [ParamsModule],
  controllers: [AppController]
})
export class AppModule {}
