package com.bench.shared

import com.bench.shared.model.CreateUser
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlin.test.Test
import kotlin.test.assertEquals

/**
 * The contract requires duplicate JSON keys to resolve last-wins on BOTH the
 * `/params/body` echo path (JsonObject) and the DB-create path (class decode).
 * This pins the server's Json config to that behavior so a kotlinx.serialization
 * version bump that changes duplicate-key handling fails here, not in production.
 */
class JsonSemanticsTest {
    private val json =
        Json {
            ignoreUnknownKeys = true
            explicitNulls = false
        }

    @Test
    fun `duplicate keys resolve last-wins when parsing a JsonObject`() {
        val obj = json.parseToJsonElement("""{"name":"first","name":"second"}""").jsonObject
        assertEquals("second", obj["name"]?.jsonPrimitive?.content)
    }

    @Test
    fun `duplicate keys resolve last-wins when decoding a class`() {
        val user = json.decodeFromString<CreateUser>("""{"name":"First","name":"Second","email":"a@b.com"}""")
        assertEquals("Second", user.name)
    }
}
