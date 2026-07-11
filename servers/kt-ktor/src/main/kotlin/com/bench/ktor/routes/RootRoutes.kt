package com.bench.ktor.routes

import io.ktor.http.ContentType
import io.ktor.server.response.respond
import io.ktor.server.response.respondText
import io.ktor.server.routing.Route
import io.ktor.server.routing.get
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

/** `GET /` → `{"hello":"world"}` (JSON); `GET /health` → `OK` (text/plain). */
fun Route.rootRoutes() {
    get("/") {
        call.respond(buildJsonObject { put("hello", "world") })
    }
    get("/health") {
        call.respondText("OK", ContentType.Text.Plain)
    }
}
