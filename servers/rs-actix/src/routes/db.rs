//! `/db/{database}/*` handlers: per-backend health plus the user CRUD lifecycle.
//! `{database}` is resolved to one of the four repositories; an unknown name is a
//! `404 {"error":"not found","details":"unknown database type: <db>"}` on the CRUD
//! routes and a `503` text body on the health route (mirrors rs-axum / the Go
//! server). actix style: handlers extract `web::Data` state and `web::Path`.

use actix_web::http::header::ContentType;
use actix_web::{HttpResponse, web};
use serde_json::json;
use shared::{Backend, CreateUser, UpdateUser, Validate};

use crate::error::ApiError;
use crate::state::AppState;

/// Register the `/db/*` routes (mounted under the `/db` scope in `main`).
pub fn config(cfg: &mut web::ServiceConfig) {
    cfg.service(
        web::scope("/db")
            .route("/{database}/health", web::get().to(health))
            .service(
                web::resource("/{database}/users")
                    .route(web::post().to(create_user))
                    .route(web::delete().to(delete_all_users)),
            )
            .service(
                web::resource("/{database}/users/{id}")
                    .route(web::get().to(get_user))
                    .route(web::patch().to(update_user))
                    .route(web::delete().to(delete_user)),
            )
            .route("/{database}/reset", web::delete().to(reset)),
    );
}

/// Resolve the backend or fail with the canonical unknown-database 404.
fn resolve<'a>(repos: &'a AppState, database: &str) -> Result<Backend<'a>, ApiError> {
    repos
        .resolve(database)
        .ok_or_else(|| ApiError::NotFound(format!("unknown database type: {database}")))
}

async fn health(repos: AppState, database: web::Path<String>) -> HttpResponse {
    let healthy = match repos.resolve(&database) {
        Some(backend) => backend.health_check().await.unwrap_or(false),
        None => false,
    };
    if healthy {
        HttpResponse::Ok()
            .content_type(ContentType::plaintext())
            .body("OK")
    } else {
        HttpResponse::ServiceUnavailable()
            .content_type(ContentType::plaintext())
            .body("Service Unavailable")
    }
}

async fn create_user(
    repos: AppState,
    database: web::Path<String>,
    body: web::Bytes,
) -> Result<HttpResponse, ApiError> {
    let backend = resolve(&repos, &database)?;
    let data = decode::<CreateUser>(&body)?;
    let user = backend.create(&data).await?;
    Ok(HttpResponse::Created().json(user))
}

async fn get_user(
    repos: AppState,
    path: web::Path<(String, String)>,
) -> Result<HttpResponse, ApiError> {
    let (database, id) = path.into_inner();
    let backend = resolve(&repos, &database)?;
    match backend.find_by_id(&id).await? {
        Some(user) => Ok(HttpResponse::Ok().json(user)),
        None => Err(not_found(&id)),
    }
}

async fn update_user(
    repos: AppState,
    path: web::Path<(String, String)>,
    body: web::Bytes,
) -> Result<HttpResponse, ApiError> {
    let (database, id) = path.into_inner();
    let backend = resolve(&repos, &database)?;
    let data = decode::<UpdateUser>(&body)?;
    match backend.update(&id, &data).await? {
        Some(user) => Ok(HttpResponse::Ok().json(user)),
        None => Err(not_found(&id)),
    }
}

async fn delete_user(
    repos: AppState,
    path: web::Path<(String, String)>,
) -> Result<HttpResponse, ApiError> {
    let (database, id) = path.into_inner();
    let backend = resolve(&repos, &database)?;
    if backend.delete(&id).await? {
        Ok(HttpResponse::Ok().json(json!({ "success": true })))
    } else {
        Err(not_found(&id))
    }
}

async fn delete_all_users(
    repos: AppState,
    database: web::Path<String>,
) -> Result<HttpResponse, ApiError> {
    let backend = resolve(&repos, &database)?;
    backend.delete_all().await?;
    Ok(HttpResponse::Ok().json(json!({ "success": true })))
}

async fn reset(repos: AppState, database: web::Path<String>) -> Result<HttpResponse, ApiError> {
    let backend = resolve(&repos, &database)?;
    backend.delete_all().await?;
    Ok(HttpResponse::Ok().json(json!({ "status": "ok" })))
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
    let value: serde_json::Value =
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
