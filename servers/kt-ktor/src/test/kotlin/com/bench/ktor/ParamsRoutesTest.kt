package com.bench.ktor

import com.bench.ktor.routes.paramsRoutes
import com.bench.shared.Consts
import io.ktor.client.HttpClient
import io.ktor.client.request.forms.MultiPartFormDataContent
import io.ktor.client.request.forms.formData
import io.ktor.client.request.post
import io.ktor.client.request.setBody
import io.ktor.client.statement.HttpResponse
import io.ktor.client.statement.bodyAsText
import io.ktor.http.Headers
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.serialization.kotlinx.json.json
import io.ktor.server.application.install
import io.ktor.server.plugins.bodylimit.RequestBodyLimit
import io.ktor.server.plugins.contentnegotiation.ContentNegotiation
import io.ktor.server.plugins.statuspages.StatusPages
import io.ktor.server.routing.routing
import io.ktor.server.testing.testApplication
import kotlinx.coroutines.runBlocking
import kotlin.test.Test
import kotlin.test.assertEquals

/**
 * In-process tests for the `/params` behaviors the contract suite does not pin
 * case-by-case, all fleet-parity sensitive (Go/Rust canon):
 *  - an over-limit request body maps to the canonical 413 (not the 500 fallback);
 *  - the file type-sniff uses only the SNIFF_LEN head window, while the plain-text
 *    inspection (NUL / UTF-8) covers the full upload — so a NUL past the window is
 *    rejected as "not plain text", never as "invalid file type".
 */
class ParamsRoutesTest {
    private fun paramsApp(block: suspend (HttpClient) -> Unit) =
        testApplication {
            application {
                install(ContentNegotiation) { json(appJson) }
                install(RequestBodyLimit) { bodyLimit { Consts.MAX_REQUEST_BYTES } }
                install(StatusPages) { apiStatusPages() }
                routing { paramsRoutes() }
            }
            block(client)
        }

    private suspend fun HttpClient.postFile(
        bytes: ByteArray,
        declaredType: String?,
    ): HttpResponse =
        post("/params/file") {
            setBody(
                MultiPartFormDataContent(
                    formData {
                        append(
                            "file",
                            bytes,
                            Headers.build {
                                append(HttpHeaders.ContentDisposition, "filename=\"t.txt\"")
                                if (declaredType != null) append(HttpHeaders.ContentType, declaredType)
                            },
                        )
                    },
                ),
            )
        }

    /** 600 clean bytes, then a NUL — the NUL sits past the SNIFF_LEN (512) window. */
    private fun nulPastSniffWindow(): ByteArray = ByteArray(601) { i -> if (i < 600) 'a'.code.toByte() else 0 }

    @Test
    fun `over-limit request body maps to the canonical 413`() =
        runBlocking {
            paramsApp { client ->
                val response =
                    client.post("/params/body") { setBody(ByteArray(Consts.MAX_REQUEST_BYTES.toInt() + 1)) }
                assertEquals(HttpStatusCode.PayloadTooLarge, response.status)
                assertEquals("""{"error":"${Consts.ERR_REQUEST_TOO_LARGE}"}""", response.bodyAsText())
            }
        }

    @Test
    fun `declared text plain with a NUL past the sniff window is not plain text`() =
        runBlocking {
            paramsApp { client ->
                val response = client.postFile(nulPastSniffWindow(), "text/plain")
                assertEquals(HttpStatusCode.UnsupportedMediaType, response.status)
                assertEquals("""{"error":"${Consts.ERR_NOT_PLAIN_TEXT}"}""", response.bodyAsText())
            }
        }

    @Test
    fun `undeclared type with a clean head and a NUL past the sniff window is not plain text`() =
        runBlocking {
            paramsApp { client ->
                // The head window (512) is clean text, so the type sniff passes; the
                // full-content inspection must still reject — with ERR_NOT_PLAIN_TEXT,
                // not ERR_INVALID_FILE_TYPE (Go/Rust parity).
                val response = client.postFile(nulPastSniffWindow(), null)
                assertEquals(HttpStatusCode.UnsupportedMediaType, response.status)
                assertEquals("""{"error":"${Consts.ERR_NOT_PLAIN_TEXT}"}""", response.bodyAsText())
            }
        }

    @Test
    fun `undeclared type with a binary head is an invalid file type`() =
        runBlocking {
            paramsApp { client ->
                val response = client.postFile(byteArrayOf(0, 1, 2, 3), null)
                assertEquals(HttpStatusCode.UnsupportedMediaType, response.status)
                assertEquals("""{"error":"${Consts.ERR_INVALID_FILE_TYPE}"}""", response.bodyAsText())
            }
        }

    @Test
    fun `a plain-text upload is accepted`() =
        runBlocking {
            paramsApp { client ->
                val response = client.postFile("hello".toByteArray(), "text/plain")
                assertEquals(HttpStatusCode.OK, response.status)
                assertEquals("""{"filename":"t.txt","size":5,"content":"hello"}""", response.bodyAsText())
            }
        }
}
