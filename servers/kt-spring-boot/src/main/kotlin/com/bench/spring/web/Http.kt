package com.bench.spring.web

import com.bench.shared.Consts
import kotlinx.coroutines.reactor.awaitSingleOrNull
import org.springframework.core.io.buffer.DataBuffer
import org.springframework.core.io.buffer.DataBufferUtils
import org.springframework.web.server.ServerWebExchange
import reactor.core.publisher.Flux
import java.nio.charset.StandardCharsets

/** Read the full request body as UTF-8 text, bounded by the global 10 MiB cap. */
suspend fun readBody(exchange: ServerWebExchange): String =
    String(readAllBytes(exchange.request.body, Consts.MAX_REQUEST_BYTES.toInt()), StandardCharsets.UTF_8)

/**
 * Aggregate a DataBuffer stream into a ByteArray, releasing the joined buffer.
 * [DataBufferUtils.join] with a byte cap throws `DataBufferLimitException` past the
 * limit (mapped to 413 by the exception handler), so this is also the global
 * request-body guard — the Spring equivalent of Ktor's `RequestBodyLimit`.
 */
suspend fun readAllBytes(
    content: Flux<DataBuffer>,
    maxBytes: Int,
): ByteArray {
    val buffer = DataBufferUtils.join(content, maxBytes).awaitSingleOrNull() ?: return ByteArray(0)
    return try {
        val bytes = ByteArray(buffer.readableByteCount())
        buffer.read(bytes)
        bytes
    } finally {
        DataBufferUtils.release(buffer)
    }
}

// JS Number.MAX_SAFE_INTEGER — integer query/form values outside ±SAFE_INT fall back
// to their default, matching the other servers' safe-int parsing.
private const val SAFE_INT: Long = (1L shl 53) - 1

/** Parse a trimmed integer within the JS safe-int range, else null (→ caller default). */
fun parseSafeInt(raw: String): Long? {
    val value = raw.trim().toLongOrNull() ?: return null
    return if (value in -SAFE_INT..SAFE_INT) value else null
}
