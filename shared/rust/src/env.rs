//! Environment contract shared by every server (mirrors `shared/go/config`).
//!
//! `default_port` is the one setting that differs per framework (each server's
//! canonical port); everything else is identical infrastructure config, read
//! from the same env-var names as the Go/TS/Python servers.

use std::env;

/// Fully-resolved runtime configuration.
#[derive(Debug, Clone)]
pub struct Env {
    pub env: String,
    pub host: String,
    pub port: u16,
    pub postgres_url: String,
    pub mongodb_url: String,
    pub mongodb_db: String,
    pub redis_url: String,
    pub cassandra_contact_points: Vec<String>,
    pub cassandra_local_dc: String,
    pub cassandra_keyspace: String,
    pub jwt_secret: String,
}

impl Env {
    /// True in production mode — the caller disables request logging here.
    #[must_use]
    pub fn is_prod(&self) -> bool {
        self.env == "prod"
    }

    /// Build the config from process environment, falling back to shared dev
    /// defaults. `default_port` supplies this framework's canonical port.
    #[must_use]
    pub fn load(default_port: u16) -> Self {
        let env = env::var("ENV")
            .ok()
            .filter(|v| !v.is_empty())
            .unwrap_or_else(|| "dev".to_string());

        // `localhost` is normalised to `0.0.0.0` so the container binds on all
        // interfaces, matching the Go server's HOST handling.
        let host = match env::var("HOST").ok().filter(|v| !v.is_empty()) {
            Some(h) if h == "localhost" => "0.0.0.0".to_string(),
            Some(h) => h,
            None => "0.0.0.0".to_string(),
        };

        let port = env::var("PORT")
            .ok()
            .and_then(|p| p.parse::<u16>().ok())
            .unwrap_or(default_port);

        Self {
            env,
            host,
            port,
            postgres_url: var_or(
                "POSTGRES_URL",
                "postgres://postgres:postgres@localhost:5432/benchmarks",
            ),
            mongodb_url: var_or("MONGODB_URL", "mongodb://localhost:27017"),
            mongodb_db: var_or("MONGODB_DB", "benchmarks"),
            redis_url: var_or("REDIS_URL", "redis://localhost:6379"),
            cassandra_contact_points: parse_contact_points(&var_or(
                "CASSANDRA_CONTACT_POINTS",
                "localhost",
            )),
            cassandra_local_dc: var_or("CASSANDRA_LOCAL_DATACENTER", "datacenter1"),
            cassandra_keyspace: var_or("CASSANDRA_KEYSPACE", "benchmarks"),
            // Shared HS256 secret for the web suite; dev default must match the
            // other languages' shared env modules and the contract harness.
            jwt_secret: var_or("JWT_SECRET", "benchmarks-shared-jwt-secret-dev-default"),
        }
    }
}

fn var_or(key: &str, default: &str) -> String {
    env::var(key)
        .ok()
        .filter(|v| !v.is_empty())
        .unwrap_or_else(|| default.to_string())
}

fn parse_contact_points(value: &str) -> Vec<String> {
    value
        .split(',')
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .map(str::to_string)
        .collect()
}
