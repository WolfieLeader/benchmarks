package com.bench.ktor

import com.bench.shared.Consts
import io.ktor.http.HttpStatusCode
import io.ktor.server.plugins.statuspages.StatusPagesConfig
import io.ktor.server.response.respond
import kotlinx.serialization.Serializable

/** The uniform error body: `{"error": string, "details"?: string}`. */
@Serializable
data class ErrorBody(
    val error: String,
    val details: String? = null,
)

/**
 * A handled API error carrying its HTTP status + contract error string. Thrown from
 * handlers and rendered by StatusPages (kotlin.md item 30/48: `throw` reserved for
 * genuine 4xx/5xx mapping, not internal branching).
 */
sealed class ApiException(
    val status: HttpStatusCode,
    val error: String,
    val details: String? = null,
    cause: Throwable? = null,
) : Exception(error, cause) {
    class BadRequest(
        error: String,
        details: String? = null,
        cause: Throwable? = null,
    ) : ApiException(HttpStatusCode.BadRequest, error, details, cause)

    class NotFound(
        details: String,
    ) : ApiException(HttpStatusCode.NotFound, Consts.ERR_NOT_FOUND, details)

    class Unauthorized(
        error: String,
    ) : ApiException(HttpStatusCode.Unauthorized, error)

    class UnsupportedMediaType(
        error: String,
    ) : ApiException(HttpStatusCode.UnsupportedMediaType, error)

    class PayloadTooLarge(
        error: String,
    ) : ApiException(HttpStatusCode.PayloadTooLarge, error)
}

/**
 * StatusPages wiring: every [ApiException] renders the uniform body; any other
 * throwable (a DB/driver failure) collapses to a 500 with no detail leak. Only
 * `exception<>{}` handlers are used — never `status{}{}`, which silently overwrites
 * an explicit `respond()` for the same code (kotlin.md item 26).
 */
fun StatusPagesConfig.apiStatusPages() {
    exception<ApiException> { call, cause ->
        call.respond(cause.status, ErrorBody(cause.error, cause.details))
    }
    exception<Throwable> { call, _ ->
        call.respond(HttpStatusCode.InternalServerError, ErrorBody(Consts.ERR_INTERNAL))
    }
}
