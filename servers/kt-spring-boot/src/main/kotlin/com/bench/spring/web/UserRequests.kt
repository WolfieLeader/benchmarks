package com.bench.spring.web

import com.bench.shared.Consts
import com.bench.shared.model.CreateUser
import com.bench.shared.model.UpdateUser
import com.bench.shared.model.Validation
import com.bench.spring.appJson
import com.bench.spring.error.ApiException
import kotlinx.serialization.SerializationException

/**
 * Decode + validate the user CRUD bodies. Both a decode failure (malformed JSON, wrong
 * type, missing required field) and a shared-`Validation` rule breach surface as the
 * same `400 {"error":"invalid JSON body"}` (fleet canon, matching kt-ktor's DbRoutes).
 */
fun decodeCreate(text: String): CreateUser {
    val data =
        try {
            appJson.decodeFromString<CreateUser>(text)
        } catch (e: SerializationException) {
            throw ApiException.BadRequest(Consts.ERR_INVALID_JSON, e.message ?: "invalid payload", e)
        }
    Validation.validateCreate(data)?.let { throw ApiException.BadRequest(Consts.ERR_INVALID_JSON, it) }
    return data
}

fun decodeUpdate(text: String): UpdateUser {
    val data =
        try {
            appJson.decodeFromString<UpdateUser>(text)
        } catch (e: SerializationException) {
            throw ApiException.BadRequest(Consts.ERR_INVALID_JSON, e.message ?: "invalid payload", e)
        }
    Validation.validateUpdate(data)?.let { throw ApiException.BadRequest(Consts.ERR_INVALID_JSON, it) }
    return data
}
