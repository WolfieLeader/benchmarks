package com.bench.shared.db

import com.bench.shared.model.CreateUser
import com.bench.shared.model.UpdateUser
import com.bench.shared.model.User
import com.zaxxer.hikari.HikariConfig
import com.zaxxer.hikari.HikariDataSource
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.net.URI
import java.sql.ResultSet
import java.sql.Types

/**
 * Postgres repository — HikariCP + pgjdbc. Blocking JDBC calls run on
 * `Dispatchers.IO` so they never park a Ktor coroutine's dispatcher thread. The
 * pool is pinned to 50 connections (fairness canon — matches the Go/Rust/TS
 * Postgres pools). The `users` table is created by the DB init migration, never
 * by the server.
 */
class PostgresRepository private constructor(
    private val dataSource: HikariDataSource,
) : UserRepository {
    override suspend fun create(data: CreateUser): User =
        withContext(Dispatchers.IO) {
            val id = Ids.v7()
            dataSource.connection.use { conn ->
                conn
                    .prepareStatement(
                        "INSERT INTO users (id, name, email, favorite_number) VALUES (?, ?, ?, ?)",
                    ).use { st ->
                        st.setObject(1, id)
                        st.setString(2, data.name)
                        st.setString(3, data.email)
                        setFavoriteNumber(st, 4, data.favoriteNumber)
                        st.executeUpdate()
                    }
            }
            User(id.toString(), data.name, data.email, data.favoriteNumber)
        }

    override suspend fun findById(id: String): User? =
        withContext(Dispatchers.IO) {
            val uuid = Ids.parse(id) ?: return@withContext null
            dataSource.connection.use { conn ->
                conn
                    .prepareStatement(
                        "SELECT id, name, email, favorite_number FROM users WHERE id = ?",
                    ).use { st ->
                        st.setObject(1, uuid)
                        st.executeQuery().use { rs -> if (rs.next()) rs.toUser() else null }
                    }
            }
        }

    override suspend fun update(
        id: String,
        data: UpdateUser,
    ): User? {
        if (data.isEmpty) return findById(id)
        val uuid = Ids.parse(id) ?: return null
        return withContext(Dispatchers.IO) {
            dataSource.connection.use { conn ->
                conn
                    .prepareStatement(
                        "UPDATE users SET name = COALESCE(?, name), email = COALESCE(?, email), " +
                            "favorite_number = COALESCE(?, favorite_number) WHERE id = ? " +
                            "RETURNING id, name, email, favorite_number",
                    ).use { st ->
                        st.setString(1, data.name)
                        st.setString(2, data.email)
                        setFavoriteNumber(st, 3, data.favoriteNumber)
                        st.setObject(4, uuid)
                        st.executeQuery().use { rs -> if (rs.next()) rs.toUser() else null }
                    }
            }
        }
    }

    override suspend fun delete(id: String): Boolean =
        withContext(Dispatchers.IO) {
            val uuid = Ids.parse(id) ?: return@withContext false
            dataSource.connection.use { conn ->
                conn.prepareStatement("DELETE FROM users WHERE id = ?").use { st ->
                    st.setObject(1, uuid)
                    st.executeUpdate() > 0
                }
            }
        }

    override suspend fun deleteAll(): Unit =
        withContext(Dispatchers.IO) {
            dataSource.connection.use { conn ->
                conn.prepareStatement("DELETE FROM users").use { it.executeUpdate() }
            }
        }

    override suspend fun healthCheck(): Boolean =
        withContext(Dispatchers.IO) {
            dataSource.connection.use { conn ->
                conn.prepareStatement("SELECT 1").use { it.executeQuery().use { rs -> rs.next() } }
            }
        }

    override fun close() = dataSource.close()

    companion object {
        private const val POOL_SIZE = 50
        private const val DEFAULT_PORT = 5432

        fun connect(url: String): PostgresRepository {
            // The env URL is libpq style (postgres://user:pass@host:port/db); pgjdbc
            // needs a jdbc: URL plus separate credentials.
            val uri = URI(url)
            val credentials = uri.userInfo?.split(":", limit = 2)
            val port = if (uri.port == -1) DEFAULT_PORT else uri.port
            val database = uri.path.trimStart('/')
            val config =
                HikariConfig().apply {
                    jdbcUrl = "jdbc:postgresql://${uri.host}:$port/$database"
                    username = credentials?.getOrNull(0)
                    password = credentials?.getOrNull(1)
                    maximumPoolSize = POOL_SIZE
                }
            return PostgresRepository(HikariDataSource(config))
        }

        private fun setFavoriteNumber(
            statement: java.sql.PreparedStatement,
            index: Int,
            value: Int?,
        ) {
            if (value != null) statement.setInt(index, value) else statement.setNull(index, Types.INTEGER)
        }
    }
}

private fun ResultSet.toUser(): User {
    val favoriteNumber = getInt("favorite_number").let { if (wasNull()) null else it }
    return User(
        id = getObject("id").toString(),
        name = getString("name"),
        email = getString("email"),
        favoriteNumber = favoriteNumber,
    )
}
