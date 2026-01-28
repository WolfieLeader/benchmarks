import { randomUUIDv7 } from "bun";
import { Client } from "cassandra-driver";
import type { CreateUser, UpdateUser, User, UserRepository } from "./repo";

export type CassandraConfig = {
  contactPoints: string[];
  localDataCenter: string;
  keyspace: string;
};

export class CassandraUserRepository implements UserRepository {
  private client: Client;
  private ready: Promise<void> | null = null;

  constructor(config: CassandraConfig) {
    this.client = new Client({
      contactPoints: config.contactPoints,
      localDataCenter: config.localDataCenter,
      keyspace: config.keyspace
    });
  }

  private async connect(): Promise<void> {
    if (!this.ready) {
      this.ready = this.client.connect().then(() => this.ensureSchema());
    }
    await this.ready;
  }

  private async ensureSchema(): Promise<void> {
    await this.client.execute("CREATE TABLE IF NOT EXISTS users (id uuid PRIMARY KEY, name text, email text)");
  }

  async create(data: CreateUser): Promise<User> {
    await this.connect();
    const id = randomUUIDv7();
    await this.client.execute("INSERT INTO users (id, name, email) VALUES (?, ?, ?)", [id, data.name, data.email], {
      prepare: true
    });
    return { id, name: data.name, email: data.email };
  }

  async findById(id: string): Promise<User | null> {
    await this.connect();
    const result = await this.client.execute("SELECT id, name, email FROM users WHERE id = ?", [id], { prepare: true });
    if (result.rowLength === 0) {
      return null;
    }

    const row = result.rows[0];
    return { id: row.id.toString(), name: row.name as string, email: row.email as string };
  }

  async update(id: string, data: UpdateUser): Promise<User | null> {
    await this.connect();
    const existing = await this.findById(id);
    if (!existing) {
      return null;
    }

    await this.client.execute("UPDATE users SET name = ?, email = ? WHERE id = ?", [data.name, data.email, id], {
      prepare: true
    });

    return { id, name: data.name, email: data.email };
  }

  async delete(id: string): Promise<boolean> {
    await this.connect();
    const existing = await this.findById(id);
    if (!existing) {
      return false;
    }

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
  }
}
