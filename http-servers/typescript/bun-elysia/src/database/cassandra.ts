import { randomUUIDv7 } from "bun";
import { Client } from "cassandra-driver";
import type { UserRepository } from "./repository";
import { buildUser, type CreateUser, type UpdateUser, type User } from "./types";

export type CassandraConfig = {
  contactPoints: string[];
  localDataCenter: string;
  keyspace: string;
};

export class CassandraUserRepository implements UserRepository {
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
    const id = randomUUIDv7();
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
    await this.connect();
    try {
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
