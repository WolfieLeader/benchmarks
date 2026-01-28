import { Redis } from "ioredis";
import type { UserRepository } from "./repository.ts";
import {
  buildUser,
  type CreateUser,
  type UpdateUser,
  type User,
} from "./types.ts";

export class RedisUserRepository implements UserRepository {
  private client: Redis;
  private prefix = "user:";

  constructor(connectionString: string) {
    this.client = new Redis(connectionString);
  }

  private key(id: string): string {
    return `${this.prefix}${id}`;
  }

  async create(data: CreateUser): Promise<User> {
    const id = crypto.randomUUID();
    const fields: Record<string, string> = {
      name: data.name,
      email: data.email,
    };
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
    const keys = await this.client.keys(`${this.prefix}*`);
    if (keys.length > 0) {
      await this.client.del(...keys);
    }
  }

  async healthCheck(): Promise<boolean> {
    try {
      await this.client.ping();
      return true;
    } catch {
      return false;
    }
  }

  disconnect(): Promise<void> {
    this.client.disconnect();
    return Promise.resolve();
  }
}
