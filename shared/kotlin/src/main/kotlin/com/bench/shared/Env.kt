package com.bench.shared

/**
 * Environment contract shared by every Kotlin server (mirrors `shared/rust/env`
 * and `shared/go/config`). Host/port are NOT here — Ktor's `EngineMain` reads
 * those from `application.yaml` (`$PORT`/`$HOST` overrides); this holds only the
 * infrastructure config the app reads directly.
 */
data class Env(
    val env: String,
    val postgresUrl: String,
    val mongodbUrl: String,
    val mongodbDb: String,
    val redisUrl: String,
    val cassandraContactPoints: List<String>,
    val cassandraLocalDatacenter: String,
    val cassandraKeyspace: String,
    val jwtSecret: String,
) {
    /** True in production mode — the caller disables request logging here. */
    val isProd: Boolean get() = env == "prod"

    companion object {
        fun load(): Env =
            Env(
                env = envOr("ENV", "dev"),
                postgresUrl = envOr("POSTGRES_URL", "postgres://postgres:postgres@localhost:5432/benchmarks"),
                mongodbUrl = envOr("MONGODB_URL", "mongodb://localhost:27017"),
                mongodbDb = envOr("MONGODB_DB", "benchmarks"),
                redisUrl = envOr("REDIS_URL", "redis://localhost:6379"),
                cassandraContactPoints =
                    envOr("CASSANDRA_CONTACT_POINTS", "localhost")
                        .split(",")
                        .map { it.trim() }
                        .filter { it.isNotEmpty() },
                cassandraLocalDatacenter = envOr("CASSANDRA_LOCAL_DATACENTER", "datacenter1"),
                cassandraKeyspace = envOr("CASSANDRA_KEYSPACE", "benchmarks"),
                // Shared HS256 secret for the web suite; dev default must match the
                // other languages' shared env modules and the contract harness.
                jwtSecret = envOr("JWT_SECRET", "benchmarks-shared-jwt-secret-dev-default"),
            )

        private fun envOr(
            key: String,
            default: String,
        ): String = System.getenv(key)?.takeIf { it.isNotEmpty() } ?: default
    }
}
