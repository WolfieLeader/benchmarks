import { env } from "../config/env";
import { CassandraUserRepository } from "./cassandra";
import { MongoUserRepository } from "./mongodb";
import { PostgresUserRepository } from "./postgres";
import { RedisUserRepository } from "./redis";
import type { CreateUser, UpdateUser, User } from "./types";

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

const repositories = new Map<DatabaseType, UserRepository>();

const repositoryFactories: Record<DatabaseType, () => UserRepository> = {
  postgres: () => new PostgresUserRepository(env.POSTGRES_URL),
  mongodb: () => new MongoUserRepository(env.MONGODB_URL, env.MONGODB_DB),
  redis: () => new RedisUserRepository(env.REDIS_URL),
  cassandra: () =>
    new CassandraUserRepository({
      contactPoints: env.CASSANDRA_CONTACT_POINTS,
      localDataCenter: env.CASSANDRA_LOCAL_DATACENTER,
      keyspace: env.CASSANDRA_KEYSPACE
    })
};

export function isDatabaseType(value: string): value is DatabaseType {
  return databaseTypes.includes(value as DatabaseType);
}

export function getRepository(database: DatabaseType): UserRepository {
  let repo = repositories.get(database);
  if (!repo) {
    repo = repositoryFactories[database]();
    repositories.set(database, repo);
  }
  return repo;
}

export function resolveRepository(database: string): UserRepository | null {
  if (!isDatabaseType(database)) return null;
  return getRepository(database);
}

export async function initializeDatabases(): Promise<void> {
  for (const dbType of databaseTypes) {
    const repo = getRepository(dbType);
    await repo.healthCheck();
  }
}

export async function disconnectDatabases(): Promise<void> {
  for (const repo of repositories.values()) {
    await repo.disconnect();
  }
}

export async function getAllDatabaseStatuses(): Promise<Record<string, string>> {
  const statuses: Record<string, string> = {};
  for (const dbType of databaseTypes) {
    try {
      const repo = getRepository(dbType);
      const healthy = await repo.healthCheck();
      statuses[dbType] = healthy ? "healthy" : "unhealthy";
    } catch {
      statuses[dbType] = "unhealthy";
    }
  }
  return statuses;
}
