import { z } from "zod";

const envSchema = z
  .object({
    ENV: z.enum(["dev", "prod"]).default("dev"),
    HOST: z.string().default("0.0.0.0"),
    PORT: z
      .string()
      .default("3002")
      .transform((val) => Number.parseInt(val, 10))
      .refine((val) => val >= 1 && val <= 65535, "PORT must be between 1 and 65535"),
  })
  .transform((data) => ({
    ...data,
    HOST: data.HOST === "localhost" ? "0.0.0.0" : data.HOST,
  }));

export const env = envSchema.parse(process.env);
