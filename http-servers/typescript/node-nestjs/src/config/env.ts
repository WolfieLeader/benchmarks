/** biome-ignore-all lint/style/noProcessEnv: env.ts file */
import { z } from "zod";

const zEnv = z.object({
  ENV: z.enum(["dev", "prod"]).default("dev"),
  HOST: z
    .union([z.ipv4().trim(), z.literal("localhost")])
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
    .default(3002),
  POSTGRES_URL: z.string().trim().default("postgres://postgres:postgres@localhost:5432/benchmarks"),
  MONGODB_URL: z.string().trim().default("mongodb://localhost:27017"),
  MONGODB_DB: z.string().trim().default("benchmarks"),
  REDIS_URL: z.string().trim().default("redis://localhost:6379"),
  CASSANDRA_CONTACT_POINTS: z
    .string()
    .trim()
    .default("localhost")
    .transform((value) =>
      value
        .split(",")
        .map((item) => item.trim())
        .filter(Boolean)
    ),
  CASSANDRA_KEYSPACE: z.string().trim().default("benchmarks"),
  CASSANDRA_LOCAL_DATACENTER: z.string().trim().default("datacenter1")
});

export const env = zEnv.parse(process.env);
