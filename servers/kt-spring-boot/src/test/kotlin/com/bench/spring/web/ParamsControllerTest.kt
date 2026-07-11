package com.bench.spring.web

import com.bench.shared.Consts
import com.bench.spring.appJson
import com.bench.spring.error.ApiExceptionHandler
import org.springframework.core.io.ByteArrayResource
import org.springframework.http.HttpStatus
import org.springframework.http.MediaType
import org.springframework.http.client.MultipartBodyBuilder
import org.springframework.http.codec.json.KotlinSerializationJsonDecoder
import org.springframework.http.codec.json.KotlinSerializationJsonEncoder
import org.springframework.test.web.reactive.server.WebTestClient
import org.springframework.web.reactive.function.BodyInserters
import kotlin.test.Test

/**
 * In-process tests for the `/params/file` + body-limit behaviors the contract suite
 * does not pin case-by-case, all fleet-parity sensitive (Go/Rust/kt-ktor canon):
 *  - an over-limit request body maps to the canonical 413 (not the 500 fallback);
 *  - the plain-text inspection (NUL / UTF-8) covers the FULL upload, so a NUL past the
 *    SNIFF_LEN (512) head window is still rejected as "not plain text".
 * Bound to the controller standalone (no Spring context / DBs), with the same kotlinx
 * codec the app registers so error/response bodies serialise identically.
 */
class ParamsControllerTest {
    private val client: WebTestClient =
        WebTestClient
            .bindToController(ParamsController())
            .controllerAdvice(ApiExceptionHandler())
            .httpMessageCodecs { configurer ->
                configurer.defaultCodecs().kotlinSerializationJsonDecoder(KotlinSerializationJsonDecoder(appJson))
                configurer.defaultCodecs().kotlinSerializationJsonEncoder(KotlinSerializationJsonEncoder(appJson))
            }.build()

    /** 600 clean bytes, then a NUL — the NUL sits past the SNIFF_LEN (512) window. */
    private fun nulPastSniffWindow(): ByteArray = ByteArray(601) { i -> if (i < 600) 'a'.code.toByte() else 0 }

    private fun postFile(
        bytes: ByteArray,
        declaredType: MediaType?,
    ): WebTestClient.ResponseSpec {
        val builder = MultipartBodyBuilder()
        val part = builder.part("file", ByteArrayResource(bytes)).filename("t.txt")
        if (declaredType != null) part.contentType(declaredType)
        return client
            .post()
            .uri("/params/file")
            .body(BodyInserters.fromMultipartData(builder.build()))
            .exchange()
    }

    @Test
    fun `over-limit request body maps to the canonical 413`() {
        client
            .post()
            .uri("/params/body")
            .bodyValue(ByteArray(Consts.MAX_REQUEST_BYTES.toInt() + 1))
            .exchange()
            .expectStatus()
            .isEqualTo(HttpStatus.PAYLOAD_TOO_LARGE)
            .expectBody(String::class.java)
            .isEqualTo("""{"error":"${Consts.ERR_REQUEST_TOO_LARGE}"}""")
    }

    @Test
    fun `declared text plain with a NUL past the sniff window is not plain text`() {
        postFile(nulPastSniffWindow(), MediaType.TEXT_PLAIN)
            .expectStatus()
            .isEqualTo(HttpStatus.UNSUPPORTED_MEDIA_TYPE)
            .expectBody(String::class.java)
            .isEqualTo("""{"error":"${Consts.ERR_NOT_PLAIN_TEXT}"}""")
    }

    @Test
    fun `a plain-text upload is accepted`() {
        postFile("hello".toByteArray(), MediaType.TEXT_PLAIN)
            .expectStatus()
            .isOk
            .expectBody(String::class.java)
            .isEqualTo("""{"filename":"t.txt","size":5,"content":"hello"}""")
    }
}
