import { RedisClient, randomUUIDv7 } from "bun";
import type { UserRepository } from "./repository";
import { buildUser, type CreateUser, type UpdateUser, type User } from "./types";

export class RedisUserRepository implements UserRepository {
  private client: RedisClient;
  private prefix = "user:";
  private connected = false;

  constructor(connectionString: string) {
    this.client = new RedisClient(connectionString);
  }

  private async connect(): Promise<void> {
    if (this.connected && this.client.connected) return;
    await this.client.connect();
    this.connected = true;
  }

  private key(id: string): string {
    return `${this.prefix}${id}`;
  }

  async create(data: CreateUser): Promise<User> {
    await this.connect();
    const id = randomUUIDv7();
    const fields = ["name", data.name, "email", data.email];
    if (data.favoriteNumber !== undefined) {
      fields.push("favoriteNumber", String(data.favoriteNumber));
    }
    await this.client.send("HSET", [this.key(id), ...fields]);
    return buildUser(id, data);
  }

  async findById(id: string): Promise<User | null> {
    await this.connect();
    const key = this.key(id);
    const exists = await this.client.exists(key);
    if (!exists) return null;

    const [name, email, favoriteNumber] = await this.client.hmget(key, ["name", "email", "favoriteNumber"]);
    if (!name || !email) return null;

    const user: User = { id, name, email };
    if (favoriteNumber !== null && favoriteNumber !== undefined) {
      const parsedFavoriteNumber = Number(favoriteNumber);
      if (!Number.isFinite(parsedFavoriteNumber)) return null;
      user.favoriteNumber = parsedFavoriteNumber;
    }

    return user;
  }

  async update(id: string, data: UpdateUser): Promise<User | null> {
    await this.connect();
    const key = this.key(id);

    if (!(await this.client.exists(key))) return null;

    // Build fields array with only provided values
    const fields: string[] = [];
    if (data.name !== undefined) fields.push("name", data.name);
    if (data.email !== undefined) fields.push("email", data.email);
    if (data.favoriteNumber !== undefined) {
      fields.push("favoriteNumber", String(data.favoriteNumber));
    }

    if (fields.length > 0) {
      await this.client.send("HSET", [key, ...fields]);
    }

    return this.findById(id);
  }

  async delete(id: string): Promise<boolean> {
    await this.connect();
    const key = this.key(id);
    const deleted = await this.client.del(key);
    return deleted > 0;
  }

  async deleteAll(): Promise<void> {
    await this.connect();
    const keys = await this.client.send("KEYS", [`${this.prefix}*`]);
    if (Array.isArray(keys) && keys.length > 0) {
      await this.client.send("DEL", keys);
    }
  }

  async healthCheck(): Promise<boolean> {
    try {
      await this.connect();
      await this.client.send("PING", []);
      return true;
    } catch {
      return false;
    }
  }

  async disconnect(): Promise<void> {
    this.client.close();
    this.connected = false;
  }
}
