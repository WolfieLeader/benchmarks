package com.bench.shared.db

import com.fasterxml.uuid.Generators
import java.util.UUID

/**
 * UUIDv7 generation (JUG — the only maintained v7 generator; `java.util.UUID`
 * cannot produce v7, kotlin.md item 39) and lenient id parsing. Ids are the
 * `User.id` for every non-Mongo backend (Mongo uses `ObjectId`).
 */
internal object Ids {
    private val generator = Generators.timeBasedEpochGenerator()

    fun v7(): UUID = generator.generate()

    /** Parse an id, returning null for anything unparseable ("not found", never an error). */
    fun parse(id: String): UUID? =
        try {
            UUID.fromString(id)
        } catch (_: IllegalArgumentException) {
            null
        }
}
