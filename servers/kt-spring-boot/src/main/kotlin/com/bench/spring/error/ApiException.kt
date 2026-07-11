package com.bench.spring.error

import com.bench.shared.Consts
import kotlinx.serialization.Serializable
import org.springframework.core.io.buffer.DataBufferLimitException
import org.springframework.http.HttpStatus
import org.springframework.http.ResponseEntity
import org.springframework.web.bind.annotation.ExceptionHandler
import org.springframework.web.bind.annotation.RestControllerAdvice

/** The uniform error body: `{"error": string, "details"?: string}`. */
@Serializable
data class ErrorBody(
    val error: String,
    val details: String? = null,
)

/**
 * A handled API error carrying its HTTP status + contract error string. Thrown from
 * handlers and rendered by [ApiExceptionHandler] (kotlin.md item 30: a custom
 * `ResponseEntity<ErrorBody>` from an `@ExceptionHandler`, not `ProblemDetail`). The
 * optional `cause` chains the underlying decode failure so it is not swallowed.
 */
sealed class ApiException(
    val status: HttpStatus,
    val error: String,
    val details: String? = null,
    cause: Throwable? = null,
) : RuntimeException(error, cause) {
    class BadRequest(
        error: String,
        details: String? = null,
        cause: Throwable? = null,
    ) : ApiException(HttpStatus.BAD_REQUEST, error, details, cause)

    class NotFound(
        details: String,
    ) : ApiException(HttpStatus.NOT_FOUND, Consts.ERR_NOT_FOUND, details)

    class Unauthorized(
        error: String,
    ) : ApiException(HttpStatus.UNAUTHORIZED, error)

    class UnsupportedMediaType(
        error: String,
    ) : ApiException(HttpStatus.UNSUPPORTED_MEDIA_TYPE, error)

    class PayloadTooLarge(
        error: String,
    ) : ApiException(HttpStatus.PAYLOAD_TOO_LARGE, error)
}

/**
 * Every [ApiException] renders the uniform body; the raw-body reader's over-limit
 * `DataBufferLimitException` maps to the canonical 413; any other throwable (a
 * DB/driver failure) collapses to a 500 with no detail leak.
 */
@RestControllerAdvice
class ApiExceptionHandler {
    @ExceptionHandler(ApiException::class)
    fun handle(e: ApiException) = ResponseEntity.status(e.status).body(ErrorBody(e.error, e.details))

    @ExceptionHandler(DataBufferLimitException::class)
    fun tooLarge() = ResponseEntity.status(HttpStatus.PAYLOAD_TOO_LARGE).body(ErrorBody(Consts.ERR_REQUEST_TOO_LARGE))

    @ExceptionHandler(Throwable::class)
    fun internal() = ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR).body(ErrorBody(Consts.ERR_INTERNAL))
}
