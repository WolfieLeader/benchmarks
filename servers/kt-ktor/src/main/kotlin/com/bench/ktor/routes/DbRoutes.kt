package com.bench.ktor.routes

import com.bench.ktor.ApiException
import com.bench.ktor.appJson
import com.bench.shared.Consts
import com.bench.shared.db.Repositories
import com.bench.shared.db.UserRepository
import com.bench.shared.model.CreateUser
import com.bench.shared.model.UpdateUser
import com.bench.shared.model.Validation
import io.ktor.http.ContentType
import io.ktor.http.HttpStatusCode
import io.ktor.server.application.ApplicationCall
import io.ktor.server.request.receiveText
import io.ktor.server.response.respond
import io.ktor.server.response.respondText
import io.ktor.server.routing.Route
import io.ktor.server.routing.delete
import io.ktor.server.routing.get
import io.ktor.server.routing.patch
import io.ktor.server.routing.post
import io.ktor.server.routing.route
import io.ktor.server.util.getOrFail
import kotlinx.serialization.SerializationException
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

/**
 * The `/db/{database}` routes: per-backend health plus the user CRUD lifecycle. `{database}`
 * resolves to one of the four repositories; an unknown name is a
 * `404 {"error":"not found","details":"unknown database type: <db>"}` on CRUD and a
 * `503` text body on health (mirrors the other servers).
 */
fun Route.dbRoutes(repositories: Repositories) {
    route("/db/{database}") {
        get("/health") {
            val healthy =
                repositories
                    .resolve(call.parameters.getOrFail("database"))
                    ?.let { runCatching { it.healthCheck() }.getOrDefault(false) } ?: false
            if (healthy) {
                call.respondText("OK", ContentType.Text.Plain)
            } else {
                call.respondText("Service Unavailable", ContentType.Text.Plain, HttpStatusCode.ServiceUnavailable)
            }
        }
        post("/users") {
            val user = repositories.repo(call).create(decodeCreate(call))
            call.respond(HttpStatusCode.Created, user)
        }
        delete("/users") {
            repositories.repo(call).deleteAll()
            call.respond(buildJsonObject { put("success", true) })
        }
        get("/users/{id}") {
            val id = call.parameters.getOrFail("id")
            call.respond(repositories.repo(call).findById(id) ?: throw notFoundUser(id))
        }
        patch("/users/{id}") {
            val id = call.parameters.getOrFail("id")
            call.respond(repositories.repo(call).update(id, decodeUpdate(call)) ?: throw notFoundUser(id))
        }
        delete("/users/{id}") {
            val id = call.parameters.getOrFail("id")
            if (repositories.repo(call).delete(id)) {
                call.respond(buildJsonObject { put("success", true) })
            } else {
                throw notFoundUser(id)
            }
        }
        delete("/reset") {
            repositories.repo(call).deleteAll()
            call.respond(buildJsonObject { put("status", "ok") })
        }
    }
}

private fun Repositories.repo(call: ApplicationCall): UserRepository {
    val database = call.parameters.getOrFail("database")
    return resolve(database) ?: throw ApiException.NotFound("unknown database type: $database")
}

private fun notFoundUser(id: String) = ApiException.NotFound("user with id $id not found")

private suspend fun decodeCreate(call: ApplicationCall): CreateUser {
    val data =
        try {
            appJson.decodeFromString<CreateUser>(call.receiveText())
        } catch (e: SerializationException) {
            throw ApiException.BadRequest(Consts.ERR_INVALID_JSON, e.message, e)
        }
    Validation.validateCreate(data)?.let { throw ApiException.BadRequest(Consts.ERR_INVALID_JSON, it) }
    return data
}

private suspend fun decodeUpdate(call: ApplicationCall): UpdateUser {
    val data =
        try {
            appJson.decodeFromString<UpdateUser>(call.receiveText())
        } catch (e: SerializationException) {
            throw ApiException.BadRequest(Consts.ERR_INVALID_JSON, e.message, e)
        }
    Validation.validateUpdate(data)?.let { throw ApiException.BadRequest(Consts.ERR_INVALID_JSON, it) }
    return data
}
