import { z } from "zod";

const zEnv = z.object({
  ENV: z.enum(["dev", "prod"]).default("dev"),
  HOST: z
    .union([z.url().trim(), z.ipv4().trim(), z.literal("localhost")])
    .transform((val) => (val === "localhost" ? "0.0.0.0" : val))
    .default("0.0.0.0"),
  PORT: z
    .string()
    .trim()
    .transform((val) => {
      const num = Number(val);
      if (!Number.isSafeInteger(num) || num < 1 || num > 65535) {
        throw new Error("PORT must be an integer between 1 and 65535");
      }
      return num;
    })
    .default(3006),
});

export const env = zEnv.parse(process.env);
