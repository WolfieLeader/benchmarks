//! Cassandra repository (`scylla` CQL driver — Apache-Cassandra compatible).
//!
//! Ids are `uuid` v7 values stored in the `uuid`-typed `id` column. Updates are
//! implemented as a full-row upsert (a CQL `INSERT` is an upsert): read the row,
//! apply the patch in memory, write every column back. This keeps the value list
//! fixed-arity (no dynamic `SET` list) while matching the Go/TS per-field update
//! semantics exactly.

use scylla::client::session::Session;
use scylla::client::session_builder::SessionBuilder;
use uuid::Uuid;

use crate::error::DbError;
use crate::model::{CreateUser, UpdateUser, User};

pub struct CassandraRepo {
    session: Session,
}

// scylla returns several distinct error types (execution, row parsing); funnel
// them all through one message-carrying `DbError` variant.
fn cass(err: impl std::fmt::Display) -> DbError {
    DbError::Cassandra(err.to_string())
}

impl CassandraRepo {
    pub async fn connect(contact_points: &[String], keyspace: &str) -> Result<Self, DbError> {
        let session = SessionBuilder::new()
            .known_nodes(contact_points)
            .use_keyspace(keyspace, false)
            .build()
            .await
            .map_err(cass)?;
        Ok(Self { session })
    }

    async fn upsert(
        &self,
        id: Uuid,
        name: &str,
        email: &str,
        favorite_number: Option<i32>,
    ) -> Result<(), DbError> {
        if let Some(n) = favorite_number {
            self.session
                .query_unpaged(
                    "INSERT INTO users (id, name, email, favorite_number) VALUES (?, ?, ?, ?)",
                    (id, name, email, n),
                )
                .await
                .map_err(cass)?;
        } else {
            self.session
                .query_unpaged(
                    "INSERT INTO users (id, name, email) VALUES (?, ?, ?)",
                    (id, name, email),
                )
                .await
                .map_err(cass)?;
        }
        Ok(())
    }

    pub async fn create(&self, data: &CreateUser) -> Result<User, DbError> {
        let id = Uuid::now_v7();
        self.upsert(id, &data.name, &data.email, data.favorite_number)
            .await?;
        Ok(User::from_create(id.to_string(), data))
    }

    pub async fn find_by_id(&self, id: &str) -> Result<Option<User>, DbError> {
        let Ok(uuid) = Uuid::parse_str(id) else {
            return Ok(None);
        };
        let result = self
            .session
            .query_unpaged(
                "SELECT id, name, email, favorite_number FROM users WHERE id = ?",
                (uuid,),
            )
            .await
            .map_err(cass)?;
        let rows = result.into_rows_result().map_err(cass)?;
        let row = rows
            .maybe_first_row::<(Uuid, String, String, Option<i32>)>()
            .map_err(cass)?;
        Ok(row.map(|(row_id, name, email, favorite_number)| User {
            id: row_id.to_string(),
            name,
            email,
            favorite_number,
        }))
    }

    pub async fn update(&self, id: &str, data: &UpdateUser) -> Result<Option<User>, DbError> {
        let Some(mut user) = self.find_by_id(id).await? else {
            return Ok(None);
        };
        if data.is_empty() {
            return Ok(Some(user));
        }
        if let Some(name) = &data.name {
            user.name = name.clone();
        }
        if let Some(email) = &data.email {
            user.email = email.clone();
        }
        if let Some(n) = data.favorite_number {
            user.favorite_number = Some(n);
        }
        // user.id is the stored uuid string, so this parse cannot fail.
        let Ok(uuid) = Uuid::parse_str(&user.id) else {
            return Ok(None);
        };
        self.upsert(uuid, &user.name, &user.email, user.favorite_number)
            .await?;
        Ok(Some(user))
    }

    pub async fn delete(&self, id: &str) -> Result<bool, DbError> {
        let Ok(uuid) = Uuid::parse_str(id) else {
            return Ok(false);
        };
        if self.find_by_id(id).await?.is_none() {
            return Ok(false);
        }
        self.session
            .query_unpaged("DELETE FROM users WHERE id = ?", (uuid,))
            .await
            .map_err(cass)?;
        Ok(true)
    }

    pub async fn delete_all(&self) -> Result<(), DbError> {
        self.session
            .query_unpaged("TRUNCATE users", &[])
            .await
            .map_err(cass)?;
        Ok(())
    }

    pub async fn health_check(&self) -> Result<bool, DbError> {
        self.session
            .query_unpaged("SELECT now() FROM system.local", &[])
            .await
            .map_err(cass)?;
        Ok(true)
    }
}
