//! Shared application state: the four DB repositories behind an `Arc`, injected
//! into every handler via axum's `State`. `Repositories` is immutable after
//! startup and internally synchronized, so a cheap `Arc` clone per request is all
//! that is needed — no lock (guide rule 17).

use std::sync::Arc;

use shared::Repositories;

pub type AppState = Arc<Repositories>;
