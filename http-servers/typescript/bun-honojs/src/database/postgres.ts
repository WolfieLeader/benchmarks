import { randomUUIDv7 } from "bun";
import { eq, sql } from "drizzle-orm";
import { drizzle } from "drizzle-orm/bun-sql";
import { users } from "./drizzle-schema";
import type { CreateUser, UpdateUser, User, UserRepository } from "./repo";

export class PostgresUserRepository implements UserRepository {
  private db: ReturnType<typeof drizzle>;

  constructor(connectionString: string) {
    this.db = drizzle({ connection: { url: connectionString } });
  }

  async create(data: CreateUser): Promise<User> {
    const id = randomUUIDv7();
    const [user] = await this.db.insert(users).values({ id, name: data.name, email: data.email }).returning();
    return user;
  }

  async findById(id: string): Promise<User | null> {
    const [user] = await this.db.select().from(users).where(eq(users.id, id)).limit(1);
    return user ?? null;
  }

  async update(id: string, data: UpdateUser): Promise<User | null> {
    const [user] = await this.db
      .update(users)
      .set({ name: data.name, email: data.email })
      .where(eq(users.id, id))
      .returning();
    return user ?? null;
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
    return;
  }
}
