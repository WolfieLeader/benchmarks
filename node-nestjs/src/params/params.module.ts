import { Module } from "@nestjs/common";
import { ParamsController } from "./params.controller";
import { ParamsService } from "./params.service";

@Module({
  controllers: [ParamsController],
  providers: [ParamsService],
})
export class ParamsModule {}
