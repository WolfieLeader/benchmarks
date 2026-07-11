package com.bench.shared.db

import com.bench.shared.model.CreateUser
import com.bench.shared.model.UpdateUser
import com.bench.shared.model.User

/**
 * One backend's user CRUD + health, coroutine-native (`suspend`) so the Ktor
 * handlers call it directly. Postgres wraps blocking JDBC in `Dispatchers.IO`;
 * Mongo/Redis/Cassandra use their async/coroutine driver APIs.
 */
interface UserRepository {
    suspend fun create(data: CreateUser): User

    suspend fun findById(id: String): User?

    suspend fun update(
        id: String,
        data: UpdateUser,
    ): User?

    suspend fun delete(id: String): Boolean

    suspend fun deleteAll()

    suspend fun healthCheck(): Boolean

    /** Close the driver's pool/session. Called after the server drains (post-shutdown). */
    fun close()
}
