// database/repository (contract) + shared DB-layer types: the UserRepository
// interface every driver-backed class satisfies structurally, the DatabaseType
// union, and the Cassandra connection config.

import type { CreateUser, UpdateUser, User } from "./schemas.ts";

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

export type CassandraConfig = {
  contactPoints: string[];
  localDataCenter: string;
  keyspace: string;
};
