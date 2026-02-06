import { randomUUIDv7 } from "bun";
import { eq, sql } from "drizzle-orm";
import { drizzle } from "drizzle-orm/node-postgres";
import { Pool } from "pg";
import { users } from "./drizzle-schema";
import type { UserRepository } from "./repository";
import { type CreateUser, normalizeUser, type UpdateUser, type User } from "./types";

export class PostgresUserRepository implements UserRepository {
  private pool: Pool;
  private db: ReturnType<typeof drizzle>;

  constructor(connectionString: string) {
    this.pool = new Pool({ connectionString, max: 50, idleTimeoutMillis: 30000 });
    this.db = drizzle(this.pool);
  }

  async create(data: CreateUser): Promise<User> {
    const id = randomUUIDv7();
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
