package com.bench.shared.model

import kotlinx.serialization.Serializable

/**
 * A stored user as returned on the wire. `favoriteNumber` is omitted entirely
 * when absent (the contract distinguishes absent from `0`) — the server's Json is
 * configured with `explicitNulls = false` so a null value is dropped on encode.
 */
@Serializable
data class User(
    val id: String,
    val name: String,
    val email: String,
    val favoriteNumber: Int? = null,
)

/** `POST /db/<db>/users` request body. */
@Serializable
data class CreateUser(
    val name: String,
    val email: String,
    val favoriteNumber: Int? = null,
)

/**
 * `PATCH /db/<db>/users/<id>` request body. Every field optional; decoded with a
 * lenient Json (`ignoreUnknownKeys = true`) so a wrongly-cased key is a no-op that
 * returns the unchanged row (contract canon).
 */
@Serializable
data class UpdateUser(
    val name: String? = null,
    val email: String? = null,
    val favoriteNumber: Int? = null,
) {
    /** True when no field was supplied — the update is a no-op returning the row. */
    val isEmpty: Boolean get() = name == null && email == null && favoriteNumber == null
}
