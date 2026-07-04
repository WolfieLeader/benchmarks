// database/repository (registry + injectable adapters): the portable
// ioredis-backed Redis impl is the default; Bun entries swap in a
// Bun.RedisClient impl via setRedisRepositoryFactory before
// initializeDatabases() (PLAN §3). Everything else (postgres/mongo/cassandra) is
// shared verbatim; the uuid generator (id.ts) is the other injectable.

import { CassandraUserRepository } from "./db-cassandra.ts";
import { MongoUserRepository } from "./db-mongodb.ts";
import { PostgresUserRepository } from "./db-postgres.ts";
import { RedisUserRepository } from "./db-redis.ts";
import { type DatabaseType, databaseTypes, type UserRepository } from "./db-types.ts";
import { env } from "./env.ts";

let redisRepositoryFactory: (connectionString: string) => UserRepository = (connectionString) =>
  new RedisUserRepository(connectionString);

export function setRedisRepositoryFactory(factory: (connectionString: string) => UserRepository): void {
  redisRepositoryFactory = factory;
}

const repositories = new Map<DatabaseType, UserRepository>();

const repositoryFactories: Record<DatabaseType, () => UserRepository> = {
  postgres: () => new PostgresUserRepository(env.POSTGRES_URL),
  mongodb: () => new MongoUserRepository(env.MONGODB_URL, env.MONGODB_DB),
  redis: () => redisRepositoryFactory(env.REDIS_URL),
  cassandra: () =>
    new CassandraUserRepository({
      contactPoints: env.CASSANDRA_CONTACT_POINTS,
      localDataCenter: env.CASSANDRA_LOCAL_DATACENTER,
      keyspace: env.CASSANDRA_KEYSPACE
    })
};

function getRepository(database: DatabaseType): UserRepository {
  let repo = repositories.get(database);
  if (!repo) {
    repo = repositoryFactories[database]();
    repositories.set(database, repo);
  }
  return repo;
}

export function resolveRepository(database: string): UserRepository | null {
  if (!databaseTypes.includes(database as DatabaseType)) return null;
  return getRepository(database as DatabaseType);
}

export async function initializeDatabases(): Promise<void> {
  await Promise.all(databaseTypes.map((db) => getRepository(db).healthCheck()));
}

export async function disconnectDatabases(): Promise<void> {
  await Promise.all(
    databaseTypes.map(async (db) => {
      const repo = repositories.get(db);
      if (repo) await repo.disconnect();
    })
  );
}
