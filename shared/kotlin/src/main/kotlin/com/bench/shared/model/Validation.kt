package com.bench.shared.model

/**
 * Cross-language validation canon for the user model: name non-empty, email
 * well-formed, `favoriteNumber` an integer in `[0, 100]`. Mirrors the Go
 * `validator` tags, the TS zod schema, the pydantic model, and the Rust
 * `validator` derive. Returns a human-readable failure reason (the `details`
 * string, asserted `$present` by the contract) or `null` when valid.
 */
object Validation {
    private val EMAIL = Regex("^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Za-z]{2,}$")
    private val UUID = Regex("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")
    private val FAVORITE_NUMBER_RANGE = 0..100

    fun isEmail(value: String): Boolean = EMAIL.matches(value)

    fun isUuid(value: String): Boolean = UUID.matches(value)

    fun validateCreate(user: CreateUser): String? {
        if (user.name.isEmpty()) return "name must not be empty"
        if (!isEmail(user.email)) return "email must be a valid email address"
        if (user.favoriteNumber != null && user.favoriteNumber !in FAVORITE_NUMBER_RANGE) {
            return "favoriteNumber must be between 0 and 100"
        }
        return null
    }

    fun validateUpdate(user: UpdateUser): String? {
        if (user.name != null && user.name.isEmpty()) return "name must not be empty"
        if (user.email != null && !isEmail(user.email)) return "email must be a valid email address"
        if (user.favoriteNumber != null && user.favoriteNumber !in FAVORITE_NUMBER_RANGE) {
            return "favoriteNumber must be between 0 and 100"
        }
        return null
    }
}
