package com.bench.shared.db

import com.bench.shared.model.CreateUser
import com.bench.shared.model.UpdateUser
import com.bench.shared.model.User
import com.datastax.oss.driver.api.core.CqlSession
import com.datastax.oss.driver.api.core.config.DefaultDriverOption
import com.datastax.oss.driver.api.core.config.DriverConfigLoader
import com.datastax.oss.driver.api.core.cql.SimpleStatement
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.future.await
import kotlinx.coroutines.withContext
import java.util.UUID

/**
 * Cassandra repository — the Apache `java-driver-core` (the ASF fork; the frozen
 * DataStax `com.datastax.oss` group is stuck at 4.17, kotlin.md item 38). Queries
 * run through `executeAsync(...).await()`. Ids are UUIDv7 stored in the `uuid` `id`
 * column. Updates are a full-row upsert (a CQL INSERT is an upsert): read, patch
 * in memory, write every column back — fixed-arity, matching the Go/Rust semantics.
 */
class CassandraRepository private constructor(
    private val session: CqlSession,
) : UserRepository {
    override suspend fun create(data: CreateUser): User {
        val id = Ids.v7()
        upsert(id, data.name, data.email, data.favoriteNumber)
        return User(id.toString(), data.name, data.email, data.favoriteNumber)
    }

    override suspend fun findById(id: String): User? {
        val uuid = Ids.parse(id) ?: return null
        val result =
            session
                .executeAsync(
                    SimpleStatement.newInstance(
                        "SELECT id, name, email, favorite_number FROM users WHERE id = ?",
                        uuid,
                    ),
                ).await()
        val row = result.one() ?: return null
        val favoriteNumber = if (row.isNull("favorite_number")) null else row.getInt("favorite_number")
        return User(
            id = row.getUuid("id").toString(),
            name = row.getString("name").orEmpty(),
            email = row.getString("email").orEmpty(),
            favoriteNumber = favoriteNumber,
        )
    }

    override suspend fun update(
        id: String,
        data: UpdateUser,
    ): User? {
        val existing = findById(id) ?: return null
        if (data.isEmpty) return existing
        val updated =
            existing.copy(
                name = data.name ?: existing.name,
                email = data.email ?: existing.email,
                favoriteNumber = data.favoriteNumber ?: existing.favoriteNumber,
            )
        // existing.id is the stored uuid string, so this parse cannot fail.
        upsert(UUID.fromString(updated.id), updated.name, updated.email, updated.favoriteNumber)
        return updated
    }

    override suspend fun delete(id: String): Boolean {
        val uuid = Ids.parse(id) ?: return false
        if (findById(id) == null) return false
        session.executeAsync(SimpleStatement.newInstance("DELETE FROM users WHERE id = ?", uuid)).await()
        return true
    }

    override suspend fun deleteAll() {
        session.executeAsync(SimpleStatement.newInstance("TRUNCATE users")).await()
    }

    override suspend fun healthCheck(): Boolean {
        session.executeAsync(SimpleStatement.newInstance("SELECT now() FROM system.local")).await()
        return true
    }

    override fun close() = session.close()

    private suspend fun upsert(
        id: UUID,
        name: String,
        email: String,
        favoriteNumber: Int?,
    ) {
        val statement =
            if (favoriteNumber != null) {
                SimpleStatement.newInstance(
                    "INSERT INTO users (id, name, email, favorite_number) VALUES (?, ?, ?, ?)",
                    id,
                    name,
                    email,
                    favoriteNumber,
                )
            } else {
                SimpleStatement.newInstance(
                    "INSERT INTO users (id, name, email) VALUES (?, ?, ?)",
                    id,
                    name,
                    email,
                )
            }
        session.executeAsync(statement).await()
    }

    companion object {
        private const val DEFAULT_PORT = 9042

        suspend fun connect(
            contactPoints: List<String>,
            localDatacenter: String,
            keyspace: String,
        ): CassandraRepository =
            withContext(Dispatchers.IO) {
                val loader =
                    DriverConfigLoader
                        .programmaticBuilder()
                        .withStringList(
                            DefaultDriverOption.CONTACT_POINTS,
                            contactPoints.map { if (it.contains(":")) it else "$it:$DEFAULT_PORT" },
                        ).withString(DefaultDriverOption.LOAD_BALANCING_LOCAL_DATACENTER, localDatacenter)
                        .withClass(DefaultDriverOption.ADDRESS_TRANSLATOR_CLASS, ContactPointTranslator::class.java)
                        .build()
                val session =
                    CqlSession
                        .builder()
                        .withConfigLoader(loader)
                        .withKeyspace(keyspace)
                        .build()
                CassandraRepository(session)
            }
    }
}
