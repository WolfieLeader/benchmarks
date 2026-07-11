package com.bench.spring.web

import java.nio.ByteBuffer
import java.nio.charset.CharacterCodingException

/**
 * `net/http`'s "binary data byte" set — the bytes that make Go's `DetectContentType`
 * classify content as non-text. Mirrors the Go/Rust/kt-ktor servers' type sniffing;
 * applied to the SNIFF_LEN head window only, and only when the part declares no
 * Content-Type. The hex bounds ARE the canonical set (kt-ktor/rs-axum inline them
 * identically) — naming each range endpoint would only restate the table, hence the
 * targeted suppress.
 */
@Suppress("MagicNumber")
fun looksLikeText(head: ByteArray): Boolean =
    head.none { b ->
        val u = b.toInt() and 0xFF
        u <= 0x08 || u == 0x0B || u in 0x0E..0x1A || u in 0x1C..0x1F
    }

/** Plain-text = no NUL bytes and valid UTF-8 (rejects the binary anti-sniffing fixture). */
fun looksLikePlainText(bytes: ByteArray): Boolean = bytes.none { it.toInt() == 0 } && isValidUtf8(bytes)

private fun isValidUtf8(bytes: ByteArray): Boolean =
    try {
        // A fresh decoder reports (throws on) malformed input, unlike Charset.decode.
        Charsets.UTF_8.newDecoder().decode(ByteBuffer.wrap(bytes))
        true
    } catch (_: CharacterCodingException) {
        false
    }
