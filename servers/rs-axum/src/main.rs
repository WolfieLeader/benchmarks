//! rs-axum — the axum implementation of the 16-route benchmark API.
//!
//! Idiomatic axum: a `Router` composed from typed extractors, shared state via
//! `State`, cross-cutting concerns as tower layers, and one `IntoResponse` error
//! type. Serves on the multi-threaded tokio runtime with graceful shutdown on
//! SIGINT/SIGTERM, draining in-flight requests before the DB pools are closed.
#![forbid(unsafe_code)]

mod error;
mod routes;
mod state;

use std::sync::Arc;

use axum::extract::DefaultBodyLimit;
use axum::http::StatusCode;
use axum::response::IntoResponse;
use axum::{Json, Router, routing::get};
use serde_json::json;
use shared::{Env, Repositories, consts};
use tokio::net::TcpListener;
use tokio::signal;
use tower_http::trace::TraceLayer;

use state::AppState;

/// This framework's local-dev host port (PLAN §6: Rust = 24001 rs-axum); inside
/// the container PORT is baked to the canonical 8080 (PLAN §6 rule 1).
const DEFAULT_PORT: u16 = 24001;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Load a local .env for dev; in the container all vars are baked in.
    dotenvy::dotenv().ok();
    let env = Env::load(DEFAULT_PORT);

    // Logger off in prod (repo mandate); structured request tracing in dev.
    if !env.is_prod() {
        tracing_subscriber::fmt()
            .with_env_filter(
                tracing_subscriber::EnvFilter::try_from_default_env().unwrap_or_else(|_| {
                    tracing_subscriber::EnvFilter::new("info,tower_http=debug")
                }),
            )
            .init();
    }

    let repos: AppState = Arc::new(Repositories::connect(&env).await?);
    let jwt_secret: Arc<str> = Arc::from(env.jwt_secret.as_str());
    let app = build_app(repos.clone(), jwt_secret, env.is_prod());

    let listener = TcpListener::bind((env.host.as_str(), env.port)).await?;
    println!("rs-axum listening on http://{}:{}", env.host, env.port);
    axum::serve(listener, app)
        .with_graceful_shutdown(shutdown_signal())
        .await?;

    // Drain finished (serve returned) — now close the DB pools (guide rule 32).
    repos.disconnect().await;
    Ok(())
}

fn build_app(state: AppState, jwt_secret: Arc<str>, prod: bool) -> Router {
    let app = Router::new()
        .route("/", get(root))
        .route("/health", get(health))
        .nest("/params", routes::params::router())
        .nest("/db", routes::db::router())
        .merge(routes::web::router(jwt_secret))
        .fallback(not_found)
        .with_state(state)
        // Global 10 MiB request-body cap (axum's default is only 2 MiB); the file
        // route enforces its own tighter 1 MiB limit and returns 413.
        .layer(DefaultBodyLimit::max(consts::MAX_REQUEST_BYTES));
    if prod {
        app
    } else {
        app.layer(TraceLayer::new_for_http())
    }
}

async fn root() -> Json<serde_json::Value> {
    Json(json!({ "hello": "world" }))
}

async fn health() -> &'static str {
    "OK"
}

async fn not_found() -> impl IntoResponse {
    (
        StatusCode::NOT_FOUND,
        Json(json!({ "error": consts::ERR_NOT_FOUND })),
    )
}

/// Resolve when either SIGINT (Ctrl-C) or SIGTERM (container stop) arrives.
async fn shutdown_signal() {
    let ctrl_c = async {
        signal::ctrl_c()
            .await
            .expect("failed to install Ctrl-C handler");
    };

    #[cfg(unix)]
    let terminate = async {
        signal::unix::signal(signal::unix::SignalKind::terminate())
            .expect("failed to install SIGTERM handler")
            .recv()
            .await;
    };
    #[cfg(not(unix))]
    let terminate = std::future::pending::<()>();

    tokio::select! {
        () = ctrl_c => {},
        () = terminate => {},
    }
}
