import { RedisClient, randomUUIDv7 } from "bun";
import type { CreateUser, UpdateUser, User, UserRepository } from "./repo";

export class RedisUserRepository implements UserRepository {
  private client: RedisClient;
  private prefix = "user:";

  constructor(connectionString: string) {
    this.client = new RedisClient(connectionString);
  }

  private async ensureConnected(): Promise<void> {
    if (!this.client.connected) {
      await this.client.connect();
    }
  }

  private key(id: string): string {
    return `${this.prefix}${id}`;
  }

  async create(data: CreateUser): Promise<User> {
    await this.ensureConnected();
    const id = randomUUIDv7();
    const key = this.key(id);
    await this.client.send("HSET", [key, "name", data.name, "email", data.email]);
    return { id, name: data.name, email: data.email };
  }

  async findById(id: string): Promise<User | null> {
    await this.ensureConnected();
    const key = this.key(id);
    const exists = await this.client.exists(key);
    if (!exists) {
      return null;
    }

    const [name, email] = await this.client.hmget(key, ["name", "email"]);
    if (!name || !email) {
      return null;
    }

    return { id, name, email };
  }

  async update(id: string, data: UpdateUser): Promise<User | null> {
    await this.ensureConnected();
    const key = this.key(id);
    const exists = await this.client.exists(key);
    if (!exists) {
      return null;
    }

    await this.client.send("HSET", [key, "name", data.name, "email", data.email]);
    return { id, name: data.name, email: data.email };
  }

  async delete(id: string): Promise<boolean> {
    await this.ensureConnected();
    const key = this.key(id);
    const deleted = await this.client.del(key);
    return deleted > 0;
  }

  async deleteAll(): Promise<void> {
    await this.ensureConnected();
    const keys = await this.client.send("KEYS", [`${this.prefix}*`]);
    if (Array.isArray(keys) && keys.length > 0) {
      await this.client.send("DEL", keys);
    }
  }

  async healthCheck(): Promise<boolean> {
    try {
      await this.ensureConnected();
      await this.client.send("PING", []);
      return true;
    } catch {
      return false;
    }
  }

  async disconnect(): Promise<void> {
    this.client.close();
  }
}
