//! Postgres repository (sqlx, runtime query API).
//!
//! We use sqlx's runtime `query_as`/`query` rather than the compile-time `query!`
//! macros so the crate builds with no live database and no committed `.sqlx`
//! offline cache — the contract container has no dev DB at image-build time. The
//! pool is pinned to `max_connections(50)` to match the Go/TS Postgres pools
//! (fairness canon).

use sqlx::postgres::{PgPool, PgPoolOptions};
use uuid::Uuid;

use crate::error::DbError;
use crate::model::{CreateUser, UpdateUser, User};

#[derive(sqlx::FromRow)]
struct UserRow {
    id: Uuid,
    name: String,
    email: String,
    favorite_number: Option<i32>,
}

impl From<UserRow> for User {
    fn from(row: UserRow) -> Self {
        Self {
            id: row.id.to_string(),
            name: row.name,
            email: row.email,
            favorite_number: row.favorite_number,
        }
    }
}

pub struct PostgresRepo {
    pool: PgPool,
}

impl PostgresRepo {
    pub async fn connect(url: &str) -> Result<Self, DbError> {
        let pool = PgPoolOptions::new()
            .max_connections(50)
            .connect(url)
            .await?;
        Ok(Self { pool })
    }

    pub async fn create(&self, data: &CreateUser) -> Result<User, DbError> {
        let id = Uuid::now_v7();
        let row = sqlx::query_as::<_, UserRow>(
            "INSERT INTO users (id, name, email, favorite_number) VALUES ($1, $2, $3, $4) \
             RETURNING id, name, email, favorite_number",
        )
        .bind(id)
        .bind(&data.name)
        .bind(&data.email)
        .bind(data.favorite_number)
        .fetch_one(&self.pool)
        .await?;
        Ok(row.into())
    }

    pub async fn find_by_id(&self, id: &str) -> Result<Option<User>, DbError> {
        // An unparseable id is simply "not found", never an error.
        let Ok(uuid) = Uuid::parse_str(id) else {
            return Ok(None);
        };
        let row = sqlx::query_as::<_, UserRow>(
            "SELECT id, name, email, favorite_number FROM users WHERE id = $1",
        )
        .bind(uuid)
        .fetch_optional(&self.pool)
        .await?;
        Ok(row.map(Into::into))
    }

    pub async fn update(&self, id: &str, data: &UpdateUser) -> Result<Option<User>, DbError> {
        if data.is_empty() {
            return self.find_by_id(id).await;
        }
        let Ok(uuid) = Uuid::parse_str(id) else {
            return Ok(None);
        };
        let row = sqlx::query_as::<_, UserRow>(
            "UPDATE users SET \
             name = COALESCE($2, name), \
             email = COALESCE($3, email), \
             favorite_number = COALESCE($4, favorite_number) \
             WHERE id = $1 \
             RETURNING id, name, email, favorite_number",
        )
        .bind(uuid)
        .bind(data.name.as_deref())
        .bind(data.email.as_deref())
        .bind(data.favorite_number)
        .fetch_optional(&self.pool)
        .await?;
        Ok(row.map(Into::into))
    }

    pub async fn delete(&self, id: &str) -> Result<bool, DbError> {
        let Ok(uuid) = Uuid::parse_str(id) else {
            return Ok(false);
        };
        let rows = sqlx::query("DELETE FROM users WHERE id = $1")
            .bind(uuid)
            .execute(&self.pool)
            .await?
            .rows_affected();
        Ok(rows > 0)
    }

    pub async fn delete_all(&self) -> Result<(), DbError> {
        sqlx::query("DELETE FROM users").execute(&self.pool).await?;
        Ok(())
    }

    pub async fn health_check(&self) -> Result<bool, DbError> {
        sqlx::query("SELECT 1").execute(&self.pool).await?;
        Ok(true)
    }

    pub async fn disconnect(&self) {
        self.pool.close().await;
    }
}
