package com.bench.ktor.routes

import com.bench.ktor.ApiException
import com.bench.ktor.appJson
import com.bench.shared.Consts
import io.ktor.http.ContentType
import io.ktor.http.Cookie
import io.ktor.http.CookieEncoding
import io.ktor.http.content.PartData
import io.ktor.http.content.forEachPart
import io.ktor.server.application.ApplicationCall
import io.ktor.server.request.contentType
import io.ktor.server.request.receiveMultipart
import io.ktor.server.request.receiveParameters
import io.ktor.server.request.receiveText
import io.ktor.server.response.respond
import io.ktor.server.routing.Route
import io.ktor.server.routing.get
import io.ktor.server.routing.post
import io.ktor.server.routing.route
import io.ktor.utils.io.readRemaining
import kotlinx.io.readByteArray
import kotlinx.serialization.SerializationException
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import java.nio.ByteBuffer
import java.nio.charset.CharacterCodingException

// JS Number.MAX_SAFE_INTEGER — integer query/form values outside ±SAFE_INT fall
// back to their default, matching the other servers' safe-int parsing.
private const val SAFE_INT: Long = (1L shl 53) - 1

/** The `/params` routes: query/path/header/cookie/body echoes plus form and file. */
fun Route.paramsRoutes() {
    route("/params") {
        searchRoute()
        urlRoute()
        headerRoute()
        cookieRoute()
        bodyRoute()
        formRoute()
        fileRoute()
    }
}

private fun Route.searchRoute() =
    get("/search") {
        val search =
            call.request.queryParameters["q"]
                ?.trim()
                ?.takeIf { it.isNotEmpty() } ?: "none"
        val limit =
            call.request.queryParameters["limit"]?.let(::parseSafeInt) ?: Consts.DEFAULT_LIMIT
        call.respond(
            buildJsonObject {
                put("search", search)
                put("limit", limit)
            },
        )
    }

private fun Route.urlRoute() =
    get("/url/{dynamic}") {
        call.respond(buildJsonObject { put("dynamic", call.parameters["dynamic"].orEmpty()) })
    }

private fun Route.headerRoute() =
    get("/header") {
        val value =
            call.request.headers["X-Custom-Header"]
                ?.trim()
                ?.takeIf { it.isNotEmpty() } ?: "none"
        call.respond(buildJsonObject { put("header", value) })
    }

private fun Route.cookieRoute() =
    get("/cookie") {
        val value =
            call.request.cookies["foo", CookieEncoding.RAW]
                ?.trim()
                ?.takeIf { it.isNotEmpty() } ?: "none"
        call.response.cookies.append(
            Cookie(name = "bar", value = "12345", maxAge = 10, path = "/", httpOnly = true),
        )
        call.respond(buildJsonObject { put("cookie", value) })
    }

private fun Route.bodyRoute() =
    post("/body") {
        val element =
            try {
                appJson.parseToJsonElement(call.receiveText())
            } catch (e: SerializationException) {
                throw ApiException.BadRequest(Consts.ERR_INVALID_JSON, e.message, e)
            }
        // Any non-object top-level value (array/string/number/bool/null) is rejected.
        val obj =
            element as? JsonObject
                ?: throw ApiException.BadRequest(Consts.ERR_INVALID_JSON, "expected a JSON object")
        call.respond(buildJsonObject { put("body", obj) })
    }

private fun Route.formRoute() =
    post("/form") {
        val contentType = call.request.contentType()
        val fields =
            when {
                contentType.match(ContentType.Application.FormUrlEncoded) ->
                    call.receiveParameters().entries().associate { it.key to it.value.first() }
                contentType.match(ContentType.MultiPart.FormData) -> readMultipartFields(call)
                else -> throw ApiException.BadRequest(Consts.ERR_INVALID_FORM, Consts.ERR_EXPECTED_FORM_CONTENT_TYPE)
            }
        val name = fields["name"]?.trim()?.takeIf { it.isNotEmpty() } ?: "none"
        val age = fields["age"]?.let(::parseSafeInt) ?: 0L
        call.respond(
            buildJsonObject {
                put("name", name)
                put("age", age)
            },
        )
    }

private fun Route.fileRoute() =
    post("/file") {
        if (!call.request.contentType().match(ContentType.MultiPart.FormData)) {
            throw ApiException.BadRequest(Consts.ERR_INVALID_MULTIPART, Consts.ERR_EXPECTED_MULTIPART_CONTENT_TYPE)
        }
        val upload =
            readFileUpload(call)
                ?: throw ApiException.BadRequest(Consts.ERR_FILE_NOT_FOUND, "no file field in form data")
        // Size cap first (413, like rs-axum), then the type gate: the declared part
        // Content-Type when present, else a MIME sniff over the first SNIFF_LEN bytes
        // only (mirroring Go's http.DetectContentType window). The plain-text content
        // inspection (NUL / UTF-8) then runs over the FULL bytes — Go/Rust parity, so
        // e.g. a NUL past the sniff window is still rejected, but as "not plain text".
        if (upload.bytes.size > Consts.MAX_FILE_BYTES) throw ApiException.PayloadTooLarge(Consts.ERR_FILE_SIZE_EXCEEDED)
        val declared = upload.declaredType?.lowercase()
        val head =
            if (upload.bytes.size > Consts.SNIFF_LEN) upload.bytes.copyOf(Consts.SNIFF_LEN) else upload.bytes
        when {
            declared != null && !declared.startsWith("text/plain") ->
                throw ApiException.UnsupportedMediaType(Consts.ERR_INVALID_FILE_TYPE)
            declared == null && !looksLikeText(head) ->
                throw ApiException.UnsupportedMediaType(Consts.ERR_INVALID_FILE_TYPE)
        }
        if (!looksLikePlainText(upload.bytes)) throw ApiException.UnsupportedMediaType(Consts.ERR_NOT_PLAIN_TEXT)
        call.respond(
            buildJsonObject {
                put("filename", upload.filename)
                put("size", upload.bytes.size)
                put("content", String(upload.bytes, Charsets.UTF_8))
            },
        )
    }

private fun parseSafeInt(raw: String): Long? {
    val value = raw.trim().toLongOrNull() ?: return null
    return if (value in -SAFE_INT..SAFE_INT) value else null
}

private suspend fun readMultipartFields(call: ApplicationCall): Map<String, String> {
    val fields = mutableMapOf<String, String>()
    call.receiveMultipart().forEachPart { part ->
        if (part is PartData.FormItem) part.name?.let { fields[it] = part.value }
        part.release()
    }
    return fields
}

/** A buffered upload part: its filename, declared part Content-Type, and raw bytes. */
private class Upload(
    val filename: String,
    val declaredType: String?,
    val bytes: ByteArray,
)

private suspend fun readFileUpload(call: ApplicationCall): Upload? {
    var upload: Upload? = null
    call.receiveMultipart().forEachPart { part ->
        if (upload == null && part is PartData.FileItem && part.name == "file") {
            val bytes = part.provider().readRemaining().readByteArray()
            upload = Upload(part.originalFileName.orEmpty(), part.contentType?.toString(), bytes)
        }
        part.release()
    }
    return upload
}

/**
 * `net/http`'s "binary data byte" set — the bytes that make Go's `DetectContentType`
 * classify content as non-text. Mirrors the Go/Rust servers' type sniffing; applied
 * to the SNIFF_LEN head window only, and only when the part declares no Content-Type.
 * The hex bounds ARE the canonical set (rs-axum inlines them identically) — naming
 * each range endpoint would only restate the table, hence the targeted suppress.
 */
@Suppress("MagicNumber")
private fun looksLikeText(head: ByteArray): Boolean =
    head.none { b ->
        val u = b.toInt() and 0xFF
        u <= 0x08 || u == 0x0B || u in 0x0E..0x1A || u in 0x1C..0x1F
    }

/** Plain-text = no NUL bytes and valid UTF-8 (rejects the binary anti-sniffing fixture). */
private fun looksLikePlainText(bytes: ByteArray): Boolean = bytes.none { it.toInt() == 0 } && isValidUtf8(bytes)

private fun isValidUtf8(bytes: ByteArray): Boolean =
    try {
        // A fresh decoder reports (throws on) malformed input, unlike Charset.decode.
        Charsets.UTF_8.newDecoder().decode(ByteBuffer.wrap(bytes))
        true
    } catch (_: CharacterCodingException) {
        false
    }
