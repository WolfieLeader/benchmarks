//! `shared` — framework-independent infrastructure for the Rust benchmark servers
//! (rs-axum, rs-actix): DB clients/repositories, the user model + validation
//! rules, the env contract, and the canonical constants. Routing, handlers, and
//! app wiring stay per-framework (PLAN §3: "sharing stops where idiom starts").
#![forbid(unsafe_code)]

pub mod consts;
pub mod db;
pub mod env;
pub mod error;
pub mod model;

pub use db::{Backend, Repositories};
pub use env::Env;
pub use error::DbError;
pub use model::{CreateUser, UpdateUser, User};
pub use validator::Validate;
