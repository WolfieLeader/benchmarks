//! The shared crate's typed error (guide rule 9: libraries expose a matchable
//! `thiserror` enum, never `anyhow`). Each server maps these variants to HTTP
//! status codes in its own `IntoResponse`/`ResponseError` impl; driver error
//! strings never reach a response body (guide rule 11).

/// Failure from any database backend.
#[derive(Debug, thiserror::Error)]
pub enum DbError {
    #[error(transparent)]
    Postgres(#[from] sqlx::Error),
    #[error(transparent)]
    Mongo(#[from] mongodb::error::Error),
    #[error(transparent)]
    Redis(#[from] redis::RedisError),
    // scylla splits failures across several concrete error types (session build,
    // execution, row parsing); collapse them to one message-carrying variant at
    // the repository boundary rather than deriving `From` for each.
    #[error("cassandra error: {0}")]
    Cassandra(String),
}
