//! rs-actix — the actix-web implementation of the 16-route benchmark API.
//!
//! Idiomatic actix-web: an `App` factory run per worker, shared state as
//! `web::Data` (an `Arc` built once at startup), routes grouped on `web::scope`s
//! via `ServiceConfig`, and one `ResponseError` type. Runs on the actix runtime
//! (tokio-based); `HttpServer` installs SIGINT/SIGTERM handlers and drains
//! in-flight requests before `run()` resolves, after which the DB pools close.
#![forbid(unsafe_code)]

mod error;
mod routes;
mod state;

use actix_web::http::header::ContentType;
use actix_web::middleware::{Condition, Logger};
use actix_web::{App, HttpResponse, HttpServer, web};
use serde_json::json;
use shared::{Env, Repositories, consts};

/// This framework's canonical host port (PLAN §6: Rust = 24002 rs-actix).
const DEFAULT_PORT: u16 = 24002;

#[actix_web::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Load a local .env for dev; in the container all vars are baked in.
    dotenvy::dotenv().ok();
    let env = Env::load(DEFAULT_PORT);
    let prod = env.is_prod();

    // Logger off in prod (repo mandate); actix's request Logger in dev, backed by
    // env_logger. `Condition` wraps it conditionally without changing the App type.
    if !prod {
        env_logger::Builder::from_env(env_logger::Env::default().default_filter_or("info")).init();
    }

    // Build the pool/clients ONCE, before HttpServer::new, and share via web::Data
    // (an Arc). The factory closure below runs per worker and only clones the Arc —
    // never constructs a pool per worker (guide rule 40). Pinned at 50 total conns.
    let repos = web::Data::new(Repositories::connect(&env).await?);
    // 10 MiB global request-body cap (actix's web::Bytes/Payload default is far
    // smaller); the file route enforces its own tighter 1 MiB limit and returns 413.
    let payload_cfg = web::PayloadConfig::new(consts::MAX_REQUEST_BYTES);

    let factory_data = repos.clone();
    // Shared HS256 secret for the web suite, built once and cloned (Arc) per
    // worker — the two JWT handlers extract it as web::Data<String>.
    let jwt_data = web::Data::new(env.jwt_secret.clone());
    println!("rs-actix listening on http://{}:{}", env.host, env.port);

    // Worker count (fairness — mandated escalation point, guide rule 39): we take
    // actix's default (one worker per logical CPU). This is a single OS process
    // whose worker threads share the one Arc'd pool built above — the direct analogue
    // of rs-axum's multi-threaded tokio runtime (guide rule 21) and Go's GOMAXPROCS,
    // both of which already use every core in one process. The repo's single-process
    // canon targets multi-PROCESS setups (e.g. FastAPI's uvicorn workers, normalized
    // to 1 in PLAN §5) — not threads within one process. Forcing `.workers(1)` would
    // uniquely handicap actix against the other multi-threaded runtimes, so we do not.
    // Flagged for the lead in the PR; flip to `.workers(1)` here if canon rules otherwise.
    let server = HttpServer::new(move || {
        App::new()
            .app_data(factory_data.clone())
            .app_data(jwt_data.clone())
            .app_data(payload_cfg.clone())
            .wrap(Condition::new(!prod, Logger::default()))
            .route("/", web::get().to(root))
            .route("/health", web::get().to(health))
            .configure(routes::params::config)
            .configure(routes::db::config)
            .configure(routes::web::config)
            .default_service(web::route().to(not_found))
    })
    .bind((env.host.as_str(), env.port))?
    .run();

    server.await?;

    // Drain finished (run() resolved) — now close the DB pools (guide rule 32) so
    // in-flight requests kept their database until they completed.
    repos.disconnect().await;
    Ok(())
}

async fn root() -> HttpResponse {
    HttpResponse::Ok().json(json!({ "hello": "world" }))
}

async fn health() -> HttpResponse {
    HttpResponse::Ok()
        .content_type(ContentType::plaintext())
        .body("OK")
}

async fn not_found() -> HttpResponse {
    HttpResponse::NotFound().json(json!({ "error": consts::ERR_NOT_FOUND }))
}
