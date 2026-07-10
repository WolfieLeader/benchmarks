package com.bench.shared.db

import com.bench.shared.model.CreateUser
import com.bench.shared.model.UpdateUser
import com.bench.shared.model.User
import io.lettuce.core.RedisClient
import io.lettuce.core.ScanArgs
import io.lettuce.core.ScanCursor
import io.lettuce.core.api.StatefulRedisConnection
import io.lettuce.core.api.async.RedisAsyncCommands
import kotlinx.coroutines.future.await

/**
 * Redis repository — Lettuce (the idiomatic async/coroutine-friendly client;
 * kotlin.md item 37). One shared, thread-safe multiplexed connection; commands go
 * through its async API bridged to coroutines with `.await()`. Users are stored as
 * a hash at `user:<id>`, mirroring the Go/Rust redis repositories.
 */
class RedisRepository private constructor(
    private val client: RedisClient,
    private val connection: StatefulRedisConnection<String, String>,
) : UserRepository {
    private val commands: RedisAsyncCommands<String, String> = connection.async()

    override suspend fun create(data: CreateUser): User {
        val id = Ids.v7().toString()
        val fields = linkedMapOf("name" to data.name, "email" to data.email)
        if (data.favoriteNumber != null) fields["favoriteNumber"] = data.favoriteNumber.toString()
        commands.hset(key(id), fields).await()
        return User(id, data.name, data.email, data.favoriteNumber)
    }

    override suspend fun findById(id: String): User? {
        val key = key(id)
        if (commands.exists(key).await() == 0L) return null
        val fields = commands.hgetall(key).await()
        val name = fields["name"] ?: return null
        val email = fields["email"] ?: return null
        return User(id, name, email, fields["favoriteNumber"]?.toIntOrNull())
    }

    override suspend fun update(
        id: String,
        data: UpdateUser,
    ): User? {
        val key = key(id)
        if (commands.exists(key).await() == 0L) return null
        if (!data.isEmpty) {
            val fields = linkedMapOf<String, String>()
            data.name?.let { fields["name"] = it }
            data.email?.let { fields["email"] = it }
            data.favoriteNumber?.let { fields["favoriteNumber"] = it.toString() }
            if (fields.isNotEmpty()) commands.hset(key, fields).await()
        }
        return findById(id)
    }

    override suspend fun delete(id: String): Boolean = commands.del(key(id)).await() > 0L

    override suspend fun deleteAll() {
        var cursor: ScanCursor = ScanCursor.INITIAL
        val args = ScanArgs.Builder.matches("$PREFIX*").limit(SCAN_COUNT)
        do {
            val result = commands.scan(cursor, args).await()
            val keys = result.keys
            // Lettuce's del takes a vararg; spreading a bounded (<= SCAN_COUNT) key
            // batch to delete them in one round-trip is intended, not a hot-path copy.
            @Suppress("SpreadOperator")
            if (keys.isNotEmpty()) commands.del(*keys.toTypedArray()).await()
            cursor = result
        } while (!cursor.isFinished)
    }

    override suspend fun healthCheck(): Boolean {
        commands.ping().await()
        return true
    }

    override fun close() {
        connection.close()
        client.shutdown()
    }

    companion object {
        private const val PREFIX = "user:"
        private const val SCAN_COUNT = 100L

        private fun key(id: String) = "$PREFIX$id"

        fun connect(url: String): RedisRepository {
            val client = RedisClient.create(url)
            return RedisRepository(client, client.connect())
        }
    }
}
