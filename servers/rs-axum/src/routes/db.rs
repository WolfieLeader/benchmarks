//! `/db/{database}/*` handlers: per-backend health plus the user CRUD lifecycle.
//! `{database}` is resolved to one of the four repositories; an unknown name is a
//! `404 {"error":"not found","details":"unknown database type: <db>"}` on the CRUD
//! routes and a `503` text body on the health route (mirrors the Go server).

use axum::body::Bytes;
use axum::extract::{Path, State};
use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use axum::{Json, Router, routing::delete, routing::get, routing::post};
use serde_json::{Value, json};
use shared::{Backend, CreateUser, UpdateUser, User, Validate};

use crate::error::ApiError;
use crate::state::AppState;

pub fn router() -> Router<AppState> {
    Router::new()
        .route("/{database}/health", get(health))
        .route(
            "/{database}/users",
            post(create_user).delete(delete_all_users),
        )
        .route(
            "/{database}/users/{id}",
            get(get_user).patch(update_user).delete(delete_user),
        )
        .route("/{database}/reset", delete(reset))
}

/// Resolve the backend or fail with the canonical unknown-database 404.
fn resolve<'a>(repos: &'a AppState, database: &str) -> Result<Backend<'a>, ApiError> {
    repos
        .resolve(database)
        .ok_or_else(|| ApiError::NotFound(format!("unknown database type: {database}")))
}

async fn health(State(repos): State<AppState>, Path(database): Path<String>) -> Response {
    let healthy = match repos.resolve(&database) {
        Some(backend) => backend.health_check().await.unwrap_or(false),
        None => false,
    };
    if healthy {
        (StatusCode::OK, "OK").into_response()
    } else {
        (StatusCode::SERVICE_UNAVAILABLE, "Service Unavailable").into_response()
    }
}

async fn create_user(
    State(repos): State<AppState>,
    Path(database): Path<String>,
    body: Bytes,
) -> Result<Response, ApiError> {
    let backend = resolve(&repos, &database)?;
    let data = decode::<CreateUser>(&body)?;
    let user = backend.create(&data).await?;
    Ok((StatusCode::CREATED, Json(user)).into_response())
}

async fn get_user(
    State(repos): State<AppState>,
    Path((database, id)): Path<(String, String)>,
) -> Result<Json<User>, ApiError> {
    let backend = resolve(&repos, &database)?;
    match backend.find_by_id(&id).await? {
        Some(user) => Ok(Json(user)),
        None => Err(not_found(&id)),
    }
}

async fn update_user(
    State(repos): State<AppState>,
    Path((database, id)): Path<(String, String)>,
    body: Bytes,
) -> Result<Json<User>, ApiError> {
    let backend = resolve(&repos, &database)?;
    let data = decode::<UpdateUser>(&body)?;
    match backend.update(&id, &data).await? {
        Some(user) => Ok(Json(user)),
        None => Err(not_found(&id)),
    }
}

async fn delete_user(
    State(repos): State<AppState>,
    Path((database, id)): Path<(String, String)>,
) -> Result<Json<Value>, ApiError> {
    let backend = resolve(&repos, &database)?;
    if backend.delete(&id).await? {
        Ok(Json(json!({ "success": true })))
    } else {
        Err(not_found(&id))
    }
}

async fn delete_all_users(
    State(repos): State<AppState>,
    Path(database): Path<String>,
) -> Result<Json<Value>, ApiError> {
    let backend = resolve(&repos, &database)?;
    backend.delete_all().await?;
    Ok(Json(json!({ "success": true })))
}

async fn reset(
    State(repos): State<AppState>,
    Path(database): Path<String>,
) -> Result<Json<Value>, ApiError> {
    let backend = resolve(&repos, &database)?;
    backend.delete_all().await?;
    Ok(Json(json!({ "status": "ok" })))
}

/// Decode + validate a request body, collapsing both a decode failure and a
/// validation failure into the same `400 {"error":"invalid JSON body"}`.
///
/// Decoding goes through `serde_json::Value` first so duplicate JSON keys resolve
/// last-wins (canon), matching `/params/body` and the Go/TS stacks. A struct
/// deserialized straight from the byte slice would instead reject duplicate keys.
fn decode<T>(body: &[u8]) -> Result<T, ApiError>
where
    T: serde::de::DeserializeOwned + Validate,
{
    let value: Value =
        serde_json::from_slice(body).map_err(|e| ApiError::InvalidJson(e.to_string()))?;
    let data: T =
        serde_json::from_value(value).map_err(|e| ApiError::InvalidJson(e.to_string()))?;
    data.validate()
        .map_err(|e| ApiError::InvalidJson(e.to_string()))?;
    Ok(data)
}

fn not_found(id: &str) -> ApiError {
    ApiError::NotFound(format!("user with id {id} not found"))
}
