// config/env: the single sanctioned place process.env is read. Every other
// module imports the parsed, validated `env` object from here (via the barrel).

import { z } from "zod";

const zEnv = z.object({
  ENV: z.enum(["dev", "prod"]).default("dev"),
  HOST: z
    .union([z.ipv4().trim(), z.literal("localhost")])
    .transform((val) => (val === "localhost" ? "0.0.0.0" : val))
    .default("0.0.0.0"),
  PORT: z.coerce.number().int().min(1).max(65535).default(3001),
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

// biome-ignore lint/style/noProcessEnv: env parsing is the one sanctioned place process.env is read; every other module must import `env` from @bench/shared
const rawEnv = process.env;

// Explicit annotation (required by oxc isolated-declarations, which cannot infer
// the type of a `.parse()` call expression across the compiler API it lacks on
// the TS 7 RC). The shape mirrors z.infer<typeof zEnv>; keeping it as a literal
// type here also keeps zEnv itself free of an annotation, since nothing exported
// references `typeof zEnv`.
export const env: {
  ENV: "dev" | "prod";
  HOST: string;
  PORT: number;
  POSTGRES_URL: string;
  MONGODB_URL: string;
  MONGODB_DB: string;
  REDIS_URL: string;
  CASSANDRA_CONTACT_POINTS: string[];
  CASSANDRA_KEYSPACE: string;
  CASSANDRA_LOCAL_DATACENTER: string;
} = zEnv.parse(rawEnv);
