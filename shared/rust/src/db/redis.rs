//! Redis repository (redis-rs `ConnectionManager`).
//!
//! `ConnectionManager` is a single auto-reconnecting multiplexed connection —
//! the idiomatic high-throughput async path for non-blocking commands (guide
//! rule 45). It is cheap to clone (each command clones a handle to the same
//! multiplexed connection); we never issue blocking commands. Users are stored as
//! a hash at `user:<id>`, mirroring the Go/TS redis repositories.

use std::collections::HashMap;

use redis::aio::ConnectionManager;
use uuid::Uuid;

use crate::error::DbError;
use crate::model::{CreateUser, UpdateUser, User};

const PREFIX: &str = "user:";

pub struct RedisRepo {
    manager: ConnectionManager,
}

fn key(id: &str) -> String {
    format!("{PREFIX}{id}")
}

impl RedisRepo {
    pub async fn connect(url: &str) -> Result<Self, DbError> {
        let client = redis::Client::open(url)?;
        let manager = ConnectionManager::new(client).await?;
        Ok(Self { manager })
    }

    pub async fn create(&self, data: &CreateUser) -> Result<User, DbError> {
        let id = Uuid::now_v7().to_string();
        let mut con = self.manager.clone();
        let mut cmd = redis::cmd("HSET");
        cmd.arg(key(&id))
            .arg("name")
            .arg(&data.name)
            .arg("email")
            .arg(&data.email);
        if let Some(n) = data.favorite_number {
            cmd.arg("favoriteNumber").arg(n);
        }
        cmd.query_async::<()>(&mut con).await?;
        Ok(User::from_create(id, data))
    }

    pub async fn find_by_id(&self, id: &str) -> Result<Option<User>, DbError> {
        let mut con = self.manager.clone();
        let k = key(id);
        let exists: bool = redis::cmd("EXISTS").arg(&k).query_async(&mut con).await?;
        if !exists {
            return Ok(None);
        }
        let fields: HashMap<String, String> =
            redis::cmd("HGETALL").arg(&k).query_async(&mut con).await?;
        let (Some(name), Some(email)) = (fields.get("name"), fields.get("email")) else {
            return Ok(None);
        };
        let favorite_number = fields
            .get("favoriteNumber")
            .and_then(|v| v.parse::<i32>().ok());
        Ok(Some(User {
            id: id.to_string(),
            name: name.clone(),
            email: email.clone(),
            favorite_number,
        }))
    }

    pub async fn update(&self, id: &str, data: &UpdateUser) -> Result<Option<User>, DbError> {
        let mut con = self.manager.clone();
        let k = key(id);
        let exists: bool = redis::cmd("EXISTS").arg(&k).query_async(&mut con).await?;
        if !exists {
            return Ok(None);
        }
        if !data.is_empty() {
            let mut cmd = redis::cmd("HSET");
            cmd.arg(&k);
            if let Some(name) = &data.name {
                cmd.arg("name").arg(name);
            }
            if let Some(email) = &data.email {
                cmd.arg("email").arg(email);
            }
            if let Some(n) = data.favorite_number {
                cmd.arg("favoriteNumber").arg(n);
            }
            cmd.query_async::<()>(&mut con).await?;
        }
        self.find_by_id(id).await
    }

    pub async fn delete(&self, id: &str) -> Result<bool, DbError> {
        let mut con = self.manager.clone();
        let deleted: i64 = redis::cmd("DEL").arg(key(id)).query_async(&mut con).await?;
        Ok(deleted > 0)
    }

    pub async fn delete_all(&self) -> Result<(), DbError> {
        let mut con = self.manager.clone();
        let pattern = format!("{PREFIX}*");
        let mut cursor: u64 = 0;
        loop {
            let (next, keys): (u64, Vec<String>) = redis::cmd("SCAN")
                .arg(cursor)
                .arg("MATCH")
                .arg(&pattern)
                .arg("COUNT")
                .arg(100)
                .query_async(&mut con)
                .await?;
            if !keys.is_empty() {
                redis::cmd("DEL")
                    .arg(&keys)
                    .query_async::<()>(&mut con)
                    .await?;
            }
            cursor = next;
            if cursor == 0 {
                break;
            }
        }
        Ok(())
    }

    pub async fn health_check(&self) -> Result<bool, DbError> {
        let mut con = self.manager.clone();
        redis::cmd("PING").query_async::<()>(&mut con).await?;
        Ok(true)
    }
}
