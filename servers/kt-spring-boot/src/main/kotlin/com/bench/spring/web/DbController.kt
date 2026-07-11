package com.bench.spring.web

import com.bench.shared.db.Repositories
import com.bench.shared.db.UserRepository
import com.bench.shared.model.User
import com.bench.spring.error.ApiException
import org.springframework.http.HttpStatus
import org.springframework.http.MediaType
import org.springframework.http.ResponseEntity
import org.springframework.web.bind.annotation.DeleteMapping
import org.springframework.web.bind.annotation.GetMapping
import org.springframework.web.bind.annotation.PatchMapping
import org.springframework.web.bind.annotation.PathVariable
import org.springframework.web.bind.annotation.PostMapping
import org.springframework.web.bind.annotation.RestController
import org.springframework.web.server.ServerWebExchange

/**
 * The `/db/{database}` routes: per-backend health plus the user CRUD lifecycle.
 * `{database}` resolves to one of the four repositories; an unknown name is a
 * `404 {"error":"not found","details":"unknown database type: <db>"}` on CRUD and a
 * `503` text body on health (mirrors the other servers).
 */
@RestController
class DbController(
    private val repositories: Repositories,
) {
    @GetMapping("/db/{database}/health")
    suspend fun health(
        @PathVariable database: String,
    ): ResponseEntity<String> {
        val repo = repositories.resolve(database)
        val healthy = repo?.let { runCatching { it.healthCheck() }.getOrDefault(false) } ?: false
        return if (healthy) {
            textResponse(HttpStatus.OK, "OK")
        } else {
            textResponse(HttpStatus.SERVICE_UNAVAILABLE, "Service Unavailable")
        }
    }

    @PostMapping("/db/{database}/users")
    suspend fun create(
        @PathVariable database: String,
        exchange: ServerWebExchange,
    ): ResponseEntity<User> {
        val user = repo(database).create(decodeCreate(readBody(exchange)))
        return ResponseEntity.status(HttpStatus.CREATED).body(user)
    }

    @GetMapping("/db/{database}/users/{id}")
    suspend fun read(
        @PathVariable database: String,
        @PathVariable id: String,
    ): User = repo(database).findById(id) ?: throw notFoundUser(id)

    @PatchMapping("/db/{database}/users/{id}")
    suspend fun update(
        @PathVariable database: String,
        @PathVariable id: String,
        exchange: ServerWebExchange,
    ): User = repo(database).update(id, decodeUpdate(readBody(exchange))) ?: throw notFoundUser(id)

    @DeleteMapping("/db/{database}/users/{id}")
    suspend fun delete(
        @PathVariable database: String,
        @PathVariable id: String,
    ): SuccessResponse {
        if (!repo(database).delete(id)) throw notFoundUser(id)
        return SuccessResponse(true)
    }

    @DeleteMapping("/db/{database}/users")
    suspend fun deleteAll(
        @PathVariable database: String,
    ): SuccessResponse {
        repo(database).deleteAll()
        return SuccessResponse(true)
    }

    @DeleteMapping("/db/{database}/reset")
    suspend fun reset(
        @PathVariable database: String,
    ): StatusResponse {
        repo(database).deleteAll()
        return StatusResponse("ok")
    }

    private fun repo(database: String): UserRepository =
        repositories.resolve(database) ?: throw ApiException.NotFound("unknown database type: $database")

    private fun notFoundUser(id: String) = ApiException.NotFound("user with id $id not found")

    private fun textResponse(
        status: HttpStatus,
        body: String,
    ): ResponseEntity<String> = ResponseEntity.status(status).contentType(MediaType.TEXT_PLAIN).body(body)
}
