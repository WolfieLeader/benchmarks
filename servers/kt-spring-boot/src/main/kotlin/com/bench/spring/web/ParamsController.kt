package com.bench.spring.web

import com.bench.shared.Consts
import com.bench.spring.appJson
import com.bench.spring.error.ApiException
import kotlinx.coroutines.reactor.awaitSingle
import kotlinx.serialization.SerializationException
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.springframework.http.HttpHeaders
import org.springframework.http.MediaType
import org.springframework.http.ResponseCookie
import org.springframework.http.ResponseEntity
import org.springframework.http.codec.multipart.FilePart
import org.springframework.http.codec.multipart.FormFieldPart
import org.springframework.web.bind.annotation.GetMapping
import org.springframework.web.bind.annotation.PathVariable
import org.springframework.web.bind.annotation.PostMapping
import org.springframework.web.bind.annotation.RequestHeader
import org.springframework.web.bind.annotation.RequestParam
import org.springframework.web.bind.annotation.RestController
import org.springframework.web.server.ServerWebExchange
import java.nio.charset.StandardCharsets

/** The `/params` routes: query/path/header/cookie/body echoes plus form and file. */
@RestController
class ParamsController {
    @GetMapping("/params/search")
    fun search(
        @RequestParam(required = false) q: String?,
        @RequestParam(required = false) limit: String?,
    ): SearchResponse {
        val search = q?.trim()?.takeIf { it.isNotEmpty() } ?: "none"
        val resolvedLimit = limit?.let(::parseSafeInt) ?: Consts.DEFAULT_LIMIT
        return SearchResponse(search, resolvedLimit)
    }

    @GetMapping("/params/url/{dynamic}")
    fun url(
        @PathVariable dynamic: String,
    ): UrlResponse = UrlResponse(dynamic)

    @GetMapping("/params/header")
    fun header(
        @RequestHeader(name = "X-Custom-Header", required = false) custom: String?,
    ): HeaderResponse {
        val header = custom?.trim()?.takeIf { it.isNotEmpty() } ?: "none"
        return HeaderResponse(header)
    }

    @GetMapping("/params/cookie")
    fun cookie(exchange: ServerWebExchange): CookieResponse {
        // Read the RAW Cookie header, not @CookieValue / exchange.request.cookies:
        // Netty's cookie decoder drops a value containing spaces (`foo=  spaced  `),
        // but the contract asserts such a value is trimmed to "spaced" (kt-ktor uses
        // Ktor's RAW cookie encoding for the same reason).
        val cookie = rawCookie(exchange, "foo")?.trim()?.takeIf { it.isNotEmpty() } ?: "none"
        exchange.response.addCookie(
            ResponseCookie
                .from("bar", "12345")
                .maxAge(COOKIE_MAX_AGE_SECONDS)
                .path("/")
                .httpOnly(true)
                .build(),
        )
        return CookieResponse(cookie)
    }

    @PostMapping("/params/body")
    suspend fun body(exchange: ServerWebExchange): ResponseEntity<String> {
        val element =
            try {
                appJson.parseToJsonElement(readBody(exchange))
            } catch (e: SerializationException) {
                throw ApiException.BadRequest(Consts.ERR_INVALID_JSON, e.message ?: "malformed JSON", e)
            }
        // Any non-object top-level value (array/string/number/bool/null) is rejected.
        val obj =
            element as? JsonObject
                ?: throw ApiException.BadRequest(Consts.ERR_INVALID_JSON, "expected a JSON object")
        val out = appJson.encodeToString(JsonObject.serializer(), buildJsonObject { put("body", obj) })
        return ResponseEntity.ok().contentType(MediaType.APPLICATION_JSON).body(out)
    }

    @PostMapping("/params/form")
    suspend fun form(exchange: ServerWebExchange): FormResponse {
        val fields = readFormFields(exchange)
        val name = fields["name"]?.trim()?.takeIf { it.isNotEmpty() } ?: "none"
        val age = fields["age"]?.let(::parseSafeInt) ?: 0L
        return FormResponse(name, age)
    }

    @PostMapping("/params/file")
    suspend fun file(exchange: ServerWebExchange): FileResponse {
        val contentType = exchange.request.headers.contentType
        if (contentType == null || !contentType.isCompatibleWith(MediaType.MULTIPART_FORM_DATA)) {
            throw ApiException.BadRequest(Consts.ERR_INVALID_MULTIPART, Consts.ERR_EXPECTED_MULTIPART_CONTENT_TYPE)
        }
        val filePart =
            exchange.multipartData.awaitSingle()["file"]?.firstOrNull() as? FilePart
                ?: throw ApiException.BadRequest(Consts.ERR_FILE_NOT_FOUND, "no file field in form data")
        // Size cap first (413), then the type gate: declared part Content-Type when
        // present, else a MIME sniff over the SNIFF_LEN head window only; the plain-text
        // content inspection (NUL / UTF-8) then runs over the FULL bytes (Go/Rust/kt-ktor
        // parity — a NUL past the window is rejected as "not plain text").
        val bytes = readAllBytes(filePart.content(), Consts.MAX_REQUEST_BYTES.toInt())
        if (bytes.size > Consts.MAX_FILE_BYTES) throw ApiException.PayloadTooLarge(Consts.ERR_FILE_SIZE_EXCEEDED)
        validateFileType(filePart, bytes)
        return FileResponse(filePart.filename(), bytes.size, String(bytes, StandardCharsets.UTF_8))
    }

    private suspend fun readFormFields(exchange: ServerWebExchange): Map<String, String> {
        val contentType = exchange.request.headers.contentType
        return when {
            contentType != null && contentType.isCompatibleWith(MediaType.APPLICATION_FORM_URLENCODED) ->
                exchange.formData.awaitSingle().toSingleValueMap()
            contentType != null && contentType.isCompatibleWith(MediaType.MULTIPART_FORM_DATA) ->
                exchange.multipartData
                    .awaitSingle()
                    .toSingleValueMap()
                    .mapNotNull { (key, part) -> (part as? FormFieldPart)?.let { key to it.value() } }
                    .toMap()
            else -> throw ApiException.BadRequest(Consts.ERR_INVALID_FORM, Consts.ERR_EXPECTED_FORM_CONTENT_TYPE)
        }
    }

    private fun validateFileType(
        filePart: FilePart,
        bytes: ByteArray,
    ) {
        val declared =
            filePart
                .headers()
                .contentType
                ?.toString()
                ?.lowercase()
        val head = if (bytes.size > Consts.SNIFF_LEN) bytes.copyOf(Consts.SNIFF_LEN) else bytes
        when {
            declared != null && !declared.startsWith("text/plain") ->
                throw ApiException.UnsupportedMediaType(Consts.ERR_INVALID_FILE_TYPE)
            declared == null && !looksLikeText(head) ->
                throw ApiException.UnsupportedMediaType(Consts.ERR_INVALID_FILE_TYPE)
        }
        if (!looksLikePlainText(bytes)) throw ApiException.UnsupportedMediaType(Consts.ERR_NOT_PLAIN_TEXT)
    }

    // Parse a single cookie value straight from the raw Cookie header, preserving a
    // value with embedded spaces that Netty's cookie decoder would otherwise drop.
    private fun rawCookie(
        exchange: ServerWebExchange,
        name: String,
    ): String? {
        val header = exchange.request.headers.getFirst(HttpHeaders.COOKIE) ?: return null
        return header
            .split(";")
            .map { it.trimStart() }
            .firstOrNull { it.startsWith("$name=") }
            ?.substringAfter("=")
    }

    private companion object {
        const val COOKIE_MAX_AGE_SECONDS = 10L
    }
}
