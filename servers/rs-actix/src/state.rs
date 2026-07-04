//! Shared application state: the four DB repositories, injected into every
//! handler as actix's `web::Data`. `web::Data<T>` is an `Arc<T>` internally, so
//! the single `Repositories` (built once at startup, before `HttpServer::new`) is
//! shared across every worker thread and every request — one connection pool, no
//! per-worker duplication (guide rules 17, 40). `Repositories` is internally
//! synchronized, so no lock is needed.

use actix_web::web;
use shared::Repositories;

pub type AppState = web::Data<Repositories>;
