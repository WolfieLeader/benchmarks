package com.bench.shared.db

import com.bench.shared.Env

/**
 * All four repositories, each connected once at startup and shared across every
 * request. `resolve` maps a `/db/<name>/…` path segment to its backend (or null
 * for an unknown name); `disconnect` closes every pool/session after drain.
 */
class Repositories private constructor(
    private val backends: Map<String, UserRepository>,
) {
    fun resolve(database: String): UserRepository? = backends[database]

    fun disconnect() = backends.values.forEach(UserRepository::close)

    companion object {
        suspend fun connect(env: Env): Repositories =
            Repositories(
                linkedMapOf(
                    "postgres" to PostgresRepository.connect(env.postgresUrl),
                    "mongodb" to MongoRepository.connect(env.mongodbUrl, env.mongodbDb),
                    "redis" to RedisRepository.connect(env.redisUrl),
                    "cassandra" to
                        CassandraRepository.connect(
                            env.cassandraContactPoints,
                            env.cassandraLocalDatacenter,
                            env.cassandraKeyspace,
                        ),
                ),
            )
    }
}
