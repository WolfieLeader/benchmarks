//! Database backends and the resolver that maps a `/db/<name>/…` path segment to
//! one of the four repositories.
//!
//! Dispatch is an enum (`Backend`), not a trait object: native `async fn` in
//! traits is not dyn-compatible without boxing, and the guide (rule 30) says to
//! prefer concrete types over an async trait we do not need. `Repositories` owns
//! all four concrete repos; `resolve` hands back a borrowing `Backend` that
//! forwards each call to the matching repo.

mod cassandra;
mod mongodb;
mod postgres;
mod redis;

use cassandra::CassandraRepo;
use mongodb::MongoRepo;
use postgres::PostgresRepo;
use redis::RedisRepo;

use crate::env::Env;
use crate::error::DbError;
use crate::model::{CreateUser, UpdateUser, User};

/// All four repositories, each connected once at startup and shared (behind an
/// `Arc`) across every request.
pub struct Repositories {
    postgres: PostgresRepo,
    mongo: MongoRepo,
    redis: RedisRepo,
    cassandra: CassandraRepo,
}

impl Repositories {
    /// Connect every backend. Fails loud if any pool/session cannot be built —
    /// the contract harness only starts servers once the DB stack is healthy.
    pub async fn connect(env: &Env) -> Result<Self, DbError> {
        Ok(Self {
            postgres: PostgresRepo::connect(&env.postgres_url).await?,
            mongo: MongoRepo::connect(&env.mongodb_url, &env.mongodb_db).await?,
            redis: RedisRepo::connect(&env.redis_url).await?,
            cassandra: CassandraRepo::connect(
                &env.cassandra_contact_points,
                &env.cassandra_keyspace,
            )
            .await?,
        })
    }

    /// Resolve a database name to its backend, or `None` for an unknown name.
    #[must_use]
    pub fn resolve(&self, database: &str) -> Option<Backend<'_>> {
        match database {
            "postgres" => Some(Backend::Postgres(&self.postgres)),
            "mongodb" => Some(Backend::Mongo(&self.mongo)),
            "redis" => Some(Backend::Redis(&self.redis)),
            "cassandra" => Some(Backend::Cassandra(&self.cassandra)),
            _ => None,
        }
    }

    /// Close the Postgres pool. Called after the HTTP server has drained so
    /// in-flight requests keep their database until they finish (guide rule 32).
    /// The mongo/redis/cassandra clients close on drop.
    pub async fn disconnect(&self) {
        self.postgres.disconnect().await;
    }
}

/// A borrowed handle to one backend, dispatching each operation to the concrete
/// repository.
pub enum Backend<'a> {
    Postgres(&'a PostgresRepo),
    Mongo(&'a MongoRepo),
    Redis(&'a RedisRepo),
    Cassandra(&'a CassandraRepo),
}

impl Backend<'_> {
    pub async fn create(&self, data: &CreateUser) -> Result<User, DbError> {
        match self {
            Self::Postgres(r) => r.create(data).await,
            Self::Mongo(r) => r.create(data).await,
            Self::Redis(r) => r.create(data).await,
            Self::Cassandra(r) => r.create(data).await,
        }
    }

    pub async fn find_by_id(&self, id: &str) -> Result<Option<User>, DbError> {
        match self {
            Self::Postgres(r) => r.find_by_id(id).await,
            Self::Mongo(r) => r.find_by_id(id).await,
            Self::Redis(r) => r.find_by_id(id).await,
            Self::Cassandra(r) => r.find_by_id(id).await,
        }
    }

    pub async fn update(&self, id: &str, data: &UpdateUser) -> Result<Option<User>, DbError> {
        match self {
            Self::Postgres(r) => r.update(id, data).await,
            Self::Mongo(r) => r.update(id, data).await,
            Self::Redis(r) => r.update(id, data).await,
            Self::Cassandra(r) => r.update(id, data).await,
        }
    }

    pub async fn delete(&self, id: &str) -> Result<bool, DbError> {
        match self {
            Self::Postgres(r) => r.delete(id).await,
            Self::Mongo(r) => r.delete(id).await,
            Self::Redis(r) => r.delete(id).await,
            Self::Cassandra(r) => r.delete(id).await,
        }
    }

    pub async fn delete_all(&self) -> Result<(), DbError> {
        match self {
            Self::Postgres(r) => r.delete_all().await,
            Self::Mongo(r) => r.delete_all().await,
            Self::Redis(r) => r.delete_all().await,
            Self::Cassandra(r) => r.delete_all().await,
        }
    }

    pub async fn health_check(&self) -> Result<bool, DbError> {
        match self {
            Self::Postgres(r) => r.health_check().await,
            Self::Mongo(r) => r.health_check().await,
            Self::Redis(r) => r.health_check().await,
            Self::Cassandra(r) => r.health_check().await,
        }
    }
}
