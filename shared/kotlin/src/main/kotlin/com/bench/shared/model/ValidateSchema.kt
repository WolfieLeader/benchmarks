package com.bench.shared.model

import kotlinx.serialization.Serializable

/**
 * `POST /validate` deep-nested request schema (contract web.json canon, ~4 levels).
 * Enum-like fields (`role`, `theme`) are typed as `String`, not Kotlin enums, so a
 * bad value ("superuser", "neon") is a *validation* failure rather than a decode
 * failure — the contract expects `400 {"error":"validation failed"}`, not
 * `invalid JSON body`. All fields keep their base JSON type so both the valid and
 * the invalid contract objects decode; the rules are checked by [OrderValidation].
 */
@Serializable
data class ValidateRequest(
    val user: ValidateUser,
    val items: List<ValidateItem>,
    val total: Double,
)

@Serializable
data class ValidateUser(
    val id: String,
    val email: String,
    val profile: ValidateProfile,
)

@Serializable
data class ValidateProfile(
    val age: Int,
    val role: String,
    val preferences: ValidatePreferences,
)

@Serializable
data class ValidatePreferences(
    val theme: String,
    val notifications: Boolean,
)

@Serializable
data class ValidateItem(
    val sku: String,
    val quantity: Int,
    val tags: List<String>,
)

/**
 * The `/validate` rule set: `user.id` a UUID, `user.email` well-formed, `age`
 * 0..120, `role` in {admin,user,guest}, `theme` in {light,dark}, each item's `sku`
 * non-empty and `quantity` 1..100, `total` >= 0. Returns the list of failures; an
 * empty list means the object is valid.
 */
object OrderValidation {
    private val ROLES = setOf("admin", "user", "guest")
    private val THEMES = setOf("light", "dark")
    private val AGE_RANGE = 0..120
    private val QUANTITY_RANGE = 1..100

    fun validate(request: ValidateRequest): List<String> {
        val errors = mutableListOf<String>()
        if (!Validation.isUuid(request.user.id)) errors += "user.id must be a UUID"
        if (!Validation.isEmail(request.user.email)) errors += "user.email must be a valid email"
        val profile = request.user.profile
        if (profile.age !in AGE_RANGE) errors += "user.profile.age must be in 0..120"
        if (profile.role !in ROLES) errors += "user.profile.role must be one of $ROLES"
        if (profile.preferences.theme !in THEMES) errors += "user.profile.preferences.theme must be one of $THEMES"
        request.items.forEachIndexed { index, item ->
            if (item.sku.isEmpty()) errors += "items[$index].sku must not be empty"
            if (item.quantity !in QUANTITY_RANGE) errors += "items[$index].quantity must be in 1..100"
        }
        if (request.total < 0) errors += "total must be >= 0"
        return errors
    }
}
