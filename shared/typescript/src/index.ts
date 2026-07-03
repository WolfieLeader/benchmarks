// @bench/shared — infrastructure shared across every TypeScript server
// (PLAN §3/§4): DB repositories, zod schemas, env parsing, consts/errors, and
// the injectable adapters (uuid generator, Redis repository) that let Bun entries
// keep their native edge while sharing everything else. Routing/handlers/app
// structure stay per-framework and idiomatic — they are NOT here.
//
// This is a SINGLE-FILE entry on purpose. It is built with `tsc` (TS 7 RC's
// native compiler has no programmatic API, so tsdown/rolldown-plugin-dts cannot
// emit declarations — PLAN §4's "shared uses tsdown" is not achievable on the RC).
// A multi-file `tsc` build emits a .d.ts tree whose relative re-exports the three
// runtimes resolve incompatibly: NodeNext (Node/Bun) requires `.js` specifiers in
// the .d.ts, while Deno resolves those same `.js` specifiers to the runtime file
// and loses the type-only members. Collapsing to one file removes all cross-file
// re-exports, so the emitted index.js + index.d.ts are self-contained and every
// runtime consumes the one built artifact identically.

import { Client } from "cassandra-driver";
import { eq, sql } from "drizzle-orm";
import { drizzle } from "drizzle-orm/node-postgres";
import { integer, pgTable, uuid, varchar } from "drizzle-orm/pg-core";
import { type Collection, MongoClient, ObjectId } from "mongodb";
import { Pool } from "pg";
import { Redis } from "ioredis";
import { v7 as uuidv7 } from "uuid";
import { z } from "zod";

// ── consts/defaults ─────────────────────────────────────────────────────────
export const MAX_REQUEST_BYTES = 10 * 1024 * 1024; // 10 MB
export const MAX_FILE_BYTES = 1 << 20; // 1MB
export const SNIFF_LEN = 512;
export const NULL_BYTE = 0x00;
export const DEFAULT_LIMIT = 10;

// ── consts/errors ───────────────────────────────────────────────────────────
export const INVALID_JSON_BODY = "invalid JSON body";
export const INVALID_FORM_DATA = "invalid form data";
export const EXPECTED_FORM_CONTENT_TYPE =
  "expected content-type: application/x-www-form-urlencoded or multipart/form-data";
export const INVALID_MULTIPART = "invalid multipart form data";
export const EXPECTED_MULTIPART_CONTENT_TYPE = "expected content-type: multipart/form-data";
export const FILE_NOT_FOUND = "file not found in form data";
export const NOT_FOUND = "not found";
export const FILE_SIZE_EXCEEDS = "file size exceeds limit";
export const ONLY_TEXT_PLAIN = "only text/plain files are allowed";
export const FILE_NOT_TEXT = "file does not look like plain text";
export const INTERNAL_ERROR = "internal error";

export type ErrorResponse = { error: string; details?: string };

export function makeError(error: string, detail?: unknown): ErrorResponse {
  if (detail instanceof Error) {
    return detail.message ? { error, details: detail.message } : { error };
  }
  if (typeof detail === "string" && detail) {
    return { error, details: detail };
  }
  return { error };
}

// ── config/env ──────────────────────────────────────────────────────────────
// biome-ignore lint/style/noProcessEnv: env parsing is the one place process.env is read
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

export const env = zEnv.parse(process.env);

// ── database/id ─────────────────────────────────────────────────────────────
// The default portable id generator is `uuid`'s v7. Bun entries inject
// `randomUUIDv7` via `setIdGenerator` (PLAN §3: Bun-native bits are injectable
// adapters chosen by the entrypoint). Every repository that mints ids calls
// `generateId()` so the choice is made once, at startup, from the entrypoint.
let idGenerator: () => string = uuidv7;

export function generateId(): string {
  return idGenerator();
}

export function setIdGenerator(fn: () => string): void {
  idGenerator = fn;
}

// ── database/types ──────────────────────────────────────────────────────────
const zFavoriteNumber = z.number().int().min(0).max(100);
const zOptionalFavoriteNumber = z.preprocess(
  (value) => (value === null ? undefined : value),
  zFavoriteNumber.optional()
);

export const zUser = z.object({
  id: z.string(),
  name: z.string(),
  email: z.email(),
  favoriteNumber: zOptionalFavoriteNumber
});

export type User = z.infer<typeof zUser>;

export const zCreateUser = z.object({
  name: z.string().min(1),
  email: z.email(),
  favoriteNumber: zOptionalFavoriteNumber
});

export type CreateUser = z.infer<typeof zCreateUser>;

export const zUpdateUser = z.object({
  name: z.string().min(1).optional(),
  email: z.email().optional(),
  favoriteNumber: zOptionalFavoriteNumber
});

export type UpdateUser = z.infer<typeof zUpdateUser>;

type UserRow = { id: string; name: string; email: string; favoriteNumber?: number | null };

export function normalizeUser(row: UserRow): User {
  const user: User = { id: row.id, name: row.name, email: row.email };
  if (row.favoriteNumber != null) user.favoriteNumber = row.favoriteNumber;
  return user;
}

export function buildUser(id: string, data: CreateUser): User {
  const user: User = { id, name: data.name, email: data.email };
  if (data.favoriteNumber !== undefined) user.favoriteNumber = data.favoriteNumber;
  return user;
}

// ── database/drizzle-schema ─────────────────────────────────────────────────
const users = pgTable("users", {
  id: uuid("id").primaryKey(),
  name: varchar("name", { length: 255 }).notNull(),
  email: varchar("email", { length: 255 }).notNull(),
  favoriteNumber: integer("favorite_number")
});

// ── database/repository (contract) ──────────────────────────────────────────
export interface UserRepository {
  create(data: CreateUser): Promise<User>;
  findById(id: string): Promise<User | null>;
  update(id: string, data: UpdateUser): Promise<User | null>;
  delete(id: string): Promise<boolean>;
  deleteAll(): Promise<void>;
  healthCheck(): Promise<boolean>;
  disconnect(): Promise<void>;
}

export const databaseTypes = ["postgres", "mongodb", "redis", "cassandra"] as const;
export type DatabaseType = (typeof databaseTypes)[number];

// ── database/postgres ───────────────────────────────────────────────────────
class PostgresUserRepository implements UserRepository {
  private pool: Pool;
  private db: ReturnType<typeof drizzle>;

  constructor(connectionString: string) {
    this.pool = new Pool({ connectionString, max: 50, idleTimeoutMillis: 30000 });
    this.db = drizzle({ client: this.pool });
  }

  async create(data: CreateUser): Promise<User> {
    const id = generateId();
    const values: typeof users.$inferInsert = { id, name: data.name, email: data.email };
    if (data.favoriteNumber !== undefined) values.favoriteNumber = data.favoriteNumber;
    const [user] = await this.db.insert(users).values(values).returning();
    if (!user) throw new Error("Failed to create user");
    return normalizeUser(user);
  }

  async findById(id: string): Promise<User | null> {
    const [user] = await this.db.select().from(users).where(eq(users.id, id)).limit(1);
    return user ? normalizeUser(user) : null;
  }

  async update(id: string, data: UpdateUser): Promise<User | null> {
    const updates: Partial<typeof users.$inferInsert> = {};

    if (data.name !== undefined) updates.name = data.name;
    if (data.email !== undefined) updates.email = data.email;
    if (data.favoriteNumber !== undefined) updates.favoriteNumber = data.favoriteNumber;

    if (Object.keys(updates).length === 0) {
      return this.findById(id);
    }

    const [user] = await this.db.update(users).set(updates).where(eq(users.id, id)).returning();
    return user ? normalizeUser(user) : null;
  }

  async delete(id: string): Promise<boolean> {
    const [user] = await this.db.delete(users).where(eq(users.id, id)).returning();
    return Boolean(user);
  }

  async deleteAll(): Promise<void> {
    await this.db.delete(users);
  }

  async healthCheck(): Promise<boolean> {
    try {
      await this.db.execute(sql`SELECT 1`);
      return true;
    } catch {
      return false;
    }
  }

  async disconnect(): Promise<void> {
    await this.pool.end();
  }
}

// ── database/mongodb ────────────────────────────────────────────────────────
type UserDocument = {
  _id: ObjectId;
  name: string;
  email: string;
  favoriteNumber?: number;
};

class MongoUserRepository implements UserRepository {
  private client: MongoClient;
  private dbName: string;
  private connected = false;
  private collection: Collection<UserDocument>;

  constructor(connectionString: string, dbName: string) {
    this.client = new MongoClient(connectionString);
    this.dbName = dbName;
    this.collection = this.client.db(this.dbName).collection<UserDocument>("users");
  }

  private async connect(): Promise<void> {
    if (this.connected) return;
    await this.client.connect();
    this.connected = true;
  }

  private parseObjectId(id: string): ObjectId | null {
    try {
      return new ObjectId(id);
    } catch {
      return null;
    }
  }

  private toUser(doc: UserDocument): User {
    const user: User = { id: doc._id.toString(), name: doc.name, email: doc.email };
    if (doc.favoriteNumber !== undefined) user.favoriteNumber = doc.favoriteNumber;
    return user;
  }

  async create(data: CreateUser): Promise<User> {
    await this.connect();
    const id = new ObjectId();
    const doc: UserDocument = { _id: id, name: data.name, email: data.email };
    if (data.favoriteNumber !== undefined) doc.favoriteNumber = data.favoriteNumber;
    await this.collection.insertOne(doc);
    return buildUser(id.toString(), data);
  }

  async findById(id: string): Promise<User | null> {
    await this.connect();
    const objectId = this.parseObjectId(id);
    if (!objectId) return null;

    const doc = await this.collection.findOne({ _id: objectId });
    return doc ? this.toUser(doc) : null;
  }

  async update(id: string, data: UpdateUser): Promise<User | null> {
    await this.connect();
    const objectId = this.parseObjectId(id);
    if (!objectId) return null;

    const updateFields: Record<string, unknown> = {};
    if (data.name !== undefined) updateFields.name = data.name;
    if (data.email !== undefined) updateFields.email = data.email;
    if (data.favoriteNumber !== undefined) updateFields.favoriteNumber = data.favoriteNumber;

    if (Object.keys(updateFields).length === 0) {
      return this.findById(id);
    }

    const doc = await this.collection.findOneAndUpdate(
      { _id: objectId },
      { $set: updateFields },
      { returnDocument: "after" }
    );
    return doc ? this.toUser(doc) : null;
  }

  async delete(id: string): Promise<boolean> {
    await this.connect();
    const objectId = this.parseObjectId(id);
    if (!objectId) return false;

    const result = await this.collection.deleteOne({ _id: objectId });
    return result.deletedCount > 0;
  }

  async deleteAll(): Promise<void> {
    await this.connect();
    await this.collection.deleteMany({});
  }

  async healthCheck(): Promise<boolean> {
    try {
      await this.connect();
      await this.client.db(this.dbName).command({ ping: 1 });
      return true;
    } catch {
      return false;
    }
  }

  async disconnect(): Promise<void> {
    await this.client.close();
    this.connected = false;
  }
}

// ── database/redis (portable ioredis default) ───────────────────────────────
class RedisUserRepository implements UserRepository {
  private client: Redis;
  private prefix = "user:";

  constructor(connectionString: string) {
    this.client = new Redis(connectionString);
  }

  private key(id: string): string {
    return `${this.prefix}${id}`;
  }

  async create(data: CreateUser): Promise<User> {
    const id = generateId();
    const fields: Record<string, string> = { name: data.name, email: data.email };
    if (data.favoriteNumber !== undefined) {
      fields.favoriteNumber = String(data.favoriteNumber);
    }
    await this.client.hset(this.key(id), fields);
    return buildUser(id, data);
  }

  async findById(id: string): Promise<User | null> {
    const key = this.key(id);
    const exists = await this.client.exists(key);
    if (!exists) return null;

    const data = await this.client.hgetall(key);
    if (!data.name || !data.email) return null;

    const user: User = { id, name: data.name, email: data.email };
    if (data.favoriteNumber !== undefined) {
      const parsedFavoriteNumber = Number(data.favoriteNumber);
      if (!Number.isFinite(parsedFavoriteNumber)) return null;
      user.favoriteNumber = parsedFavoriteNumber;
    }

    return user;
  }

  async update(id: string, data: UpdateUser): Promise<User | null> {
    const key = this.key(id);

    if (!(await this.client.exists(key))) return null;

    const fields: Record<string, string> = {};
    if (data.name !== undefined) fields.name = data.name;
    if (data.email !== undefined) fields.email = data.email;
    if (data.favoriteNumber !== undefined) {
      fields.favoriteNumber = String(data.favoriteNumber);
    }

    if (Object.keys(fields).length > 0) {
      await this.client.hset(key, fields);
    }

    return this.findById(id);
  }

  async delete(id: string): Promise<boolean> {
    const key = this.key(id);
    const deleted = await this.client.del(key);
    return deleted > 0;
  }

  async deleteAll(): Promise<void> {
    let cursor = "0";
    do {
      const [nextCursor, keys] = await this.client.scan(cursor, "MATCH", `${this.prefix}*`, "COUNT", 100);
      cursor = nextCursor;
      if (keys.length > 0) {
        await this.client.del(...keys);
      }
    } while (cursor !== "0");
  }

  async healthCheck(): Promise<boolean> {
    try {
      await this.client.ping();
      return true;
    } catch {
      return false;
    }
  }

  async disconnect(): Promise<void> {
    this.client.disconnect();
  }
}

// ── database/cassandra ──────────────────────────────────────────────────────
export type CassandraConfig = {
  contactPoints: string[];
  localDataCenter: string;
  keyspace: string;
};

class CassandraUserRepository implements UserRepository {
  private client: Client;
  private connected = false;

  constructor(config: CassandraConfig) {
    this.client = new Client({
      contactPoints: config.contactPoints,
      localDataCenter: config.localDataCenter,
      keyspace: config.keyspace
    });
  }

  private async connect(): Promise<void> {
    if (this.connected) return;
    await this.client.connect();
    this.connected = true;
  }

  async create(data: CreateUser): Promise<User> {
    await this.connect();
    const id = generateId();
    const hasFavorite = data.favoriteNumber !== undefined;
    const query = hasFavorite
      ? "INSERT INTO users (id, name, email, favorite_number) VALUES (?, ?, ?, ?)"
      : "INSERT INTO users (id, name, email) VALUES (?, ?, ?)";
    const params = hasFavorite ? [id, data.name, data.email, data.favoriteNumber] : [id, data.name, data.email];
    await this.client.execute(query, params, { prepare: true });
    return buildUser(id, data);
  }

  async findById(id: string): Promise<User | null> {
    await this.connect();
    const result = await this.client.execute("SELECT id, name, email, favorite_number FROM users WHERE id = ?", [id], {
      prepare: true
    });
    if (result.rowLength === 0) return null;

    const row = result.rows[0];
    if (!row) return null;
    const user: User = { id: row.id.toString(), name: row.name, email: row.email };
    if (row.favorite_number != null) user.favoriteNumber = row.favorite_number;
    return user;
  }

  async update(id: string, data: UpdateUser): Promise<User | null> {
    await this.connect();

    const existing = await this.findById(id);
    if (!existing) return null;

    const setClauses: string[] = [];
    const params: (string | number)[] = [];

    if (data.name !== undefined) {
      setClauses.push("name = ?");
      params.push(data.name);
      existing.name = data.name;
    }
    if (data.email !== undefined) {
      setClauses.push("email = ?");
      params.push(data.email);
      existing.email = data.email;
    }
    if (data.favoriteNumber !== undefined) {
      setClauses.push("favorite_number = ?");
      params.push(data.favoriteNumber);
      existing.favoriteNumber = data.favoriteNumber;
    }

    if (setClauses.length === 0) return existing;

    params.push(id);
    await this.client.execute(`UPDATE users SET ${setClauses.join(", ")} WHERE id = ?`, params, { prepare: true });
    return existing;
  }

  async delete(id: string): Promise<boolean> {
    await this.connect();
    const existing = await this.findById(id);
    if (!existing) return false;

    await this.client.execute("DELETE FROM users WHERE id = ?", [id], { prepare: true });
    return true;
  }

  async deleteAll(): Promise<void> {
    await this.connect();
    await this.client.execute("TRUNCATE users");
  }

  async healthCheck(): Promise<boolean> {
    try {
      await this.connect();
      await this.client.execute("SELECT now() FROM system.local");
      return true;
    } catch {
      return false;
    }
  }

  async disconnect(): Promise<void> {
    await this.client.shutdown();
    this.connected = false;
  }
}

// ── database/repository (registry + injectable adapters) ────────────────────
// The Redis repository is an injectable adapter (PLAN §3): the portable
// ioredis-backed impl is the default, and Bun entries swap in a `Bun.RedisClient`
// impl via `setRedisRepositoryFactory` before `initializeDatabases()`. Everything
// else (postgres/mongo/cassandra) is shared verbatim; the uuid generator is the
// other injectable, wired through `setIdGenerator`.
let redisRepositoryFactory: (connectionString: string) => UserRepository = (connectionString) =>
  new RedisUserRepository(connectionString);

export function setRedisRepositoryFactory(factory: (connectionString: string) => UserRepository): void {
  redisRepositoryFactory = factory;
}

const repositories = new Map<DatabaseType, UserRepository>();

const repositoryFactories: Record<DatabaseType, () => UserRepository> = {
  postgres: () => new PostgresUserRepository(env.POSTGRES_URL),
  mongodb: () => new MongoUserRepository(env.MONGODB_URL, env.MONGODB_DB),
  redis: () => redisRepositoryFactory(env.REDIS_URL),
  cassandra: () =>
    new CassandraUserRepository({
      contactPoints: env.CASSANDRA_CONTACT_POINTS,
      localDataCenter: env.CASSANDRA_LOCAL_DATACENTER,
      keyspace: env.CASSANDRA_KEYSPACE
    })
};

function getRepository(database: DatabaseType): UserRepository {
  let repo = repositories.get(database);
  if (!repo) {
    repo = repositoryFactories[database]();
    repositories.set(database, repo);
  }
  return repo;
}

export function resolveRepository(database: string): UserRepository | null {
  if (!databaseTypes.includes(database as DatabaseType)) return null;
  return getRepository(database as DatabaseType);
}

export async function initializeDatabases(): Promise<void> {
  await Promise.all(databaseTypes.map((db) => getRepository(db).healthCheck()));
}

export async function disconnectDatabases(): Promise<void> {
  await Promise.all(
    databaseTypes.map(async (db) => {
      const repo = repositories.get(db);
      if (repo) await repo.disconnect();
    })
  );
}
