package com.bench.ktor.routes

import com.bench.ktor.ApiException
import com.bench.ktor.appJson
import com.bench.shared.Consts
import com.bench.shared.model.OrderValidation
import com.bench.shared.model.ValidateRequest
import com.bench.shared.web.Compute
import com.bench.shared.web.Jwt
import io.ktor.server.freemarker.FreeMarkerContent
import io.ktor.server.request.receiveText
import io.ktor.server.response.respond
import io.ktor.server.routing.Route
import io.ktor.server.routing.get
import io.ktor.server.routing.post
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.Serializable
import kotlinx.serialization.SerializationException
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

@Serializable
private data class TokenResponse(
    val token: String,
)

// Contract canon for GET /html (rendered into index.ftl): greeting name, fruit
// list, and the labeled total the htmlContains assertion checks for.
private const val HTML_NAME = "Alice"
private const val HTML_TOTAL = 42
private val HTML_FRUITS = listOf("apple", "banana", "cherry")

/** The web suite: `GET /html`, `GET /jwt/sign`, `GET /jwt/verify`, `POST /validate`, `GET /compute`. */
fun Route.webRoutes(jwtSecret: String) {
    get("/html") {
        call.respond(
            FreeMarkerContent(
                "index.ftl",
                mapOf("name" to HTML_NAME, "fruits" to HTML_FRUITS, "total" to HTML_TOTAL),
            ),
        )
    }

    get("/jwt/sign") {
        call.respond(TokenResponse(Jwt.sign(jwtSecret)))
    }

    get("/jwt/verify") {
        val header = call.request.headers["Authorization"]
        if (header == null || !header.startsWith("Bearer ")) {
            throw ApiException.Unauthorized(Consts.ERR_INVALID_TOKEN)
        }
        val claims =
            Jwt.verify(jwtSecret, header.removePrefix("Bearer "))
                ?: throw ApiException.Unauthorized(Consts.ERR_INVALID_TOKEN)
        call.respond(claims)
    }

    post("/validate") {
        // Decode failures are malformed JSON / type mismatches ONLY (every schema
        // field has a zero-value default, Go canon) — so they map to the invalid-JSON
        // body error, mirroring Go's WriteBodyError; rule breaches are the
        // validator's job below.
        val request =
            try {
                appJson.decodeFromString<ValidateRequest>(call.receiveText())
            } catch (e: SerializationException) {
                throw ApiException.BadRequest(Consts.ERR_INVALID_JSON, e.message ?: "invalid payload", e)
            }
        val errors = OrderValidation.validate(request)
        if (errors.isEmpty()) {
            call.respond(buildJsonObject { put("valid", true) })
        } else {
            throw ApiException.BadRequest(Consts.ERR_VALIDATION_FAILED, errors.joinToString("; "))
        }
    }

    get("/compute") {
        val n = call.request.queryParameters["n"]?.toLongOrNull()
        if (n == null || n < 1) throw ApiException.BadRequest(Consts.ERR_INVALID_N, "n must be an integer >= 1")
        val result = withContext(Dispatchers.Default) { Compute.sha256Chain(Compute.clampRounds(n)) }
        call.respond(buildJsonObject { put("result", result) })
    }
}
