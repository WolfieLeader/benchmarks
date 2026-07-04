//! User domain types + validation rules (shared infrastructure per PLAN §3).
//!
//! The `#[validate(...)]` rules are the cross-language canon: name non-empty,
//! email well-formed, `favoriteNumber` an integer in `[0, 100]`. They mirror the
//! Go `validator` tags, the TS zod schema, and the pydantic model. Servers decode
//! then call `.validate()`; both a decode failure and a validation failure map to
//! the same `400 {"error":"invalid JSON body"}` response.

use serde::{Deserialize, Serialize};
use validator::Validate;

/// A stored user as returned on the wire. `favoriteNumber` is omitted entirely
/// when absent (the contract distinguishes absent from `0`).
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct User {
    pub id: String,
    pub name: String,
    pub email: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub favorite_number: Option<i32>,
}

impl User {
    /// Build the response user from a create request and a freshly-minted id.
    /// Mirrors Go's `BuildUser`: mongo/redis/cassandra return this rather than a
    /// read-back, so the id is supplied by the caller.
    #[must_use]
    pub fn from_create(id: String, data: &CreateUser) -> Self {
        Self {
            id,
            name: data.name.clone(),
            email: data.email.clone(),
            favorite_number: data.favorite_number,
        }
    }
}

/// `POST /db/<db>/users` request body.
#[derive(Debug, Deserialize, Validate)]
#[serde(rename_all = "camelCase")]
pub struct CreateUser {
    #[validate(length(min = 1))]
    pub name: String,
    #[validate(email)]
    pub email: String,
    #[validate(range(min = 0, max = 100))]
    pub favorite_number: Option<i32>,
}

/// `PATCH /db/<db>/users/<id>` request body. Every field optional; unknown keys
/// are ignored (no `deny_unknown_fields`) so a wrongly-cased key is a no-op.
#[derive(Debug, Deserialize, Validate)]
#[serde(rename_all = "camelCase")]
pub struct UpdateUser {
    #[validate(length(min = 1))]
    pub name: Option<String>,
    #[validate(email)]
    pub email: Option<String>,
    #[validate(range(min = 0, max = 100))]
    pub favorite_number: Option<i32>,
}

impl UpdateUser {
    /// True when no field was supplied — the update is a no-op that returns the
    /// unchanged row (contract: PATCH with only an unknown key is `200`).
    #[must_use]
    pub fn is_empty(&self) -> bool {
        self.name.is_none() && self.email.is_none() && self.favorite_number.is_none()
    }
}
