import { isIP } from "node:net";
import { z } from "zod";

function isUrl(value: string): boolean {
  try {
    new URL(value);
    return true;
  } catch {
    return false;
  }
}

function isValidHost(value: string): boolean {
  if (value === "localhost") {
    return true;
  }

  return isIP(value) !== 0 || isUrl(value);
}

const envSchema = z
  .object({
    ENV: z.enum(["dev", "prod"]).default("dev"),
    HOST: z
      .string()
      .default("0.0.0.0")
      .refine(isValidHost, {
        message: "HOST must be a valid URL, IP, or 'localhost'",
      }),
    PORT: z
      .string()
      .default("3001")
      .transform((value, ctx) => {
        if (!/^\d+$/.test(value)) {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            message: "PORT must be numeric",
          });
          return z.NEVER;
        }

        const port = Number(value);
        if (port < 1 || port > 65535) {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            message: "PORT must be between 1 and 65535",
          });
          return z.NEVER;
        }

        return port;
      }),
  })
  .transform((data) => ({
    ...data,
    HOST: data.HOST === "localhost" ? "0.0.0.0" : data.HOST,
  }));

export const env = envSchema.parse(process.env);
