import { Type, type Static } from "@sinclair/typebox";
import { Value } from "@sinclair/typebox/value";

const EnvSchema = Type.Object({
  ENV: Type.Union([Type.Literal("dev"), Type.Literal("prod")], {
    default: "dev",
  }),
  HOST: Type.String({ default: "0.0.0.0" }),
  PORT: Type.String({ default: "3003" }),
});

type EnvInput = Static<typeof EnvSchema>;

function parseEnv(): { ENV: "dev" | "prod"; HOST: string; PORT: number } {
  const input: EnvInput = Value.Default(EnvSchema, process.env) as EnvInput;

  if (!Value.Check(EnvSchema, input)) {
    throw new Error("Invalid environment variables");
  }

  const port = Number.parseInt(input.PORT, 10);
  if (port < 1 || port > 65535) {
    throw new Error("PORT must be between 1 and 65535");
  }

  return {
    ENV: input.ENV,
    HOST: input.HOST === "localhost" ? "0.0.0.0" : input.HOST,
    PORT: port,
  };
}

export const env = parseEnv();
