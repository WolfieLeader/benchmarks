import { z } from "zod";
import { env } from "~/consts/env";
import { CassandraUserRepository } from "./cassandra";
import { MongoUserRepository } from "./mongodb";
import { PostgresUserRepository } from "./postgres";
import { RedisUserRepository } from "./redis";

export interface UserRepository {
  create(data: CreateUser): Promise<User>;
  findById(id: string): Promise<User | null>;
  update(id: string, data: UpdateUser): Promise<User | null>;
  delete(id: string): Promise<boolean>;
  deleteAll(): Promise<void>;
  healthCheck(): Promise<boolean>;
  disconnect(): Promise<void>;
}

// Repository resolver

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
  const existing = repositories.get(database);
  if (existing) {
    return existing;
  }

  const repository = repositoryFactories[database]();
  repositories.set(database, repository);
  return repository;
}

export function resolveRepository(database: string): UserRepository | null {
  if (!isDatabaseType(database)) {
    return null;
  }

  return getRepository(database);
}

// CRUD

export const zUser = z.object({
  id: z.string(),
  name: z.string(),
  email: z.email()
});

export type User = z.infer<typeof zUser>;

export const zCreateUser = z.object({
  name: z.string().min(1),
  email: z.email()
});

export type CreateUser = z.infer<typeof zCreateUser>;

export const zUpdateUser = z.object({
  name: z.string().min(1),
  email: z.email()
});

export type UpdateUser = z.infer<typeof zUpdateUser>;
