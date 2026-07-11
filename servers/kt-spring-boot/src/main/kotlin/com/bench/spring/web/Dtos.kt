package com.bench.spring.web

import kotlinx.serialization.Serializable

/**
 * Fixed-shape response bodies. Each is `@Serializable` so Spring 7's kotlinx codec
 * (gated on the annotation via KotlinDetector) encodes it with the app-wide Json —
 * the dynamic `/params/body` echo is the only route that serialises by hand.
 */

@Serializable
data class HelloResponse(
    val hello: String,
)

@Serializable
data class SearchResponse(
    val search: String,
    val limit: Long,
)

@Serializable
data class UrlResponse(
    val dynamic: String,
)

@Serializable
data class HeaderResponse(
    val header: String,
)

@Serializable
data class CookieResponse(
    val cookie: String,
)

@Serializable
data class FormResponse(
    val name: String,
    val age: Long,
)

@Serializable
data class FileResponse(
    val filename: String,
    val size: Int,
    val content: String,
)

@Serializable
data class SuccessResponse(
    val success: Boolean,
)

@Serializable
data class StatusResponse(
    val status: String,
)

@Serializable
data class TokenResponse(
    val token: String,
)

@Serializable
data class ValidResponse(
    val valid: Boolean,
)

@Serializable
data class ComputeResponse(
    val result: String,
)
