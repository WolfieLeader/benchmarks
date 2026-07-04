// database/postgres: drizzle-over-postgres.js repository. The connection pool is
// pinned at max: 50 — the cross-language fairness canon (typescript.md; PLAN.md
// audit) — via postgres.js's `max` option; `idle_timeout` (seconds) mirrors the
// former pg `idleTimeoutMillis: 30000`.

import { eq, sql } from "drizzle-orm";
import { integer, pgTable, uuid, varchar } from "drizzle-orm/pg-core";
import { drizzle } from "drizzle-orm/postgres-js";
import postgres from "postgres";
import type { UserRepository } from "./db-types.ts";
import { generateId } from "./id.ts";
import { type CreateUser, normalizeUser, type UpdateUser, type User } from "./schemas.ts";

const users = pgTable("users", {
  id: uuid("id").primaryKey(),
  name: varchar("name", { length: 255 }).notNull(),
  email: varchar("email", { length: 255 }).notNull(),
  favoriteNumber: integer("favorite_number")
});

export class PostgresUserRepository implements UserRepository {
  private sql: ReturnType<typeof postgres>;
  private db: ReturnType<typeof drizzle>;

  constructor(connectionString: string) {
    // onnotice: postgres.js prints server NOTICEs to stdout by default,
    // violating the logger-off-in-prod convention — silence them.
    this.sql = postgres(connectionString, { max: 50, idle_timeout: 30, onnotice: () => {} });
    this.db = drizzle({ client: this.sql });
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
    await this.sql.end();
  }
}
