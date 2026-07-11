package com.bench.shared

import com.bench.shared.model.OrderValidation
import com.bench.shared.model.ValidateItem
import com.bench.shared.model.ValidatePreferences
import com.bench.shared.model.ValidateProfile
import com.bench.shared.model.ValidateRequest
import com.bench.shared.model.ValidateUser
import kotlinx.serialization.json.Json
import kotlin.test.Test
import kotlin.test.assertTrue

/**
 * Pins the `/validate` rules the contract does not exercise case-by-case, all
 * Go-canon parity:
 *  - `items` is `required,min=1` — an empty list fails validation;
 *  - every schema field is zero-value-defaulted (decode never fails on an omitted
 *    field; the validator decides) — so an omitted `age` (-> 0) PASSES while an
 *    omitted `role` (-> "") fails. Lead ruling in the zig retrofit: the canon
 *    schema range-checks fields but does not require them.
 */
class OrderValidationTest {
    private val json = Json { ignoreUnknownKeys = true }

    private fun validRequest(items: List<ValidateItem>) =
        ValidateRequest(
            user =
                ValidateUser(
                    id = "5f8b7a4e-8d1c-4a5e-9b2f-3c6d7e8f9a0b",
                    email = "jane@example.com",
                    profile =
                        ValidateProfile(
                            age = 30,
                            role = "admin",
                            preferences = ValidatePreferences(theme = "dark", notifications = true),
                        ),
                ),
            items = items,
            total = 10.0,
        )

    @Test
    fun `a valid request produces no errors`() {
        val errors = OrderValidation.validate(validRequest(listOf(ValidateItem("SKU-1", 2, listOf("a")))))
        assertTrue(errors.isEmpty(), "expected no errors, got $errors")
    }

    @Test
    fun `an empty items list fails validation`() {
        val errors = OrderValidation.validate(validRequest(emptyList()))
        assertTrue(errors.any { "items" in it }, "expected an items error, got $errors")
    }

    @Test
    fun `a body omitting age decodes and validates (age is range-checked, not required)`() {
        // The zig retrofit's canon fixture: valid except `age` is absent (-> 0, in range).
        val body =
            """{"user":{"id":"3f1a2b3c-4d5e-6f70-8192-a3b4c5d6e7f8","email":"alice@conformance-suite.com",""" +
                """"profile":{"role":"admin","preferences":{"theme":"dark","notifications":true}}},""" +
                """"items":[{"sku":"SKU-1","quantity":2,"tags":["new","featured"]}],"total":42.5}"""
        val errors = OrderValidation.validate(json.decodeFromString<ValidateRequest>(body))
        assertTrue(errors.isEmpty(), "expected no errors, got $errors")
    }

    @Test
    fun `an empty object decodes to zero values and fails validation`() {
        val errors = OrderValidation.validate(json.decodeFromString<ValidateRequest>("{}"))
        assertTrue(errors.isNotEmpty(), "zero-value request must fail (id/email/role/theme/items)")
    }
}
