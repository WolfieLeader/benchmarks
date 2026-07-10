package com.bench.shared.web

import com.bench.shared.Consts
import java.security.MessageDigest
import java.util.HexFormat

/**
 * `GET /compute?n=` iterative SHA-256 chain: apply SHA-256 to the fixed seed bytes
 * `benchmark` n times and return the lowercase hex digest. Must match the Go
 * conformance runner's `sha256Chain` byte-for-byte (each round hashes the previous
 * raw digest bytes, not its hex string).
 */
object Compute {
    /** Clamp `n` to the cap: values above [Consts.COMPUTE_MAX_ROUNDS] are rounded down. */
    fun clampRounds(n: Long): Int = minOf(n, Consts.COMPUTE_MAX_ROUNDS).toInt()

    fun sha256Chain(rounds: Int): String {
        val digest = MessageDigest.getInstance("SHA-256")
        var state = Consts.COMPUTE_SEED.toByteArray(Charsets.UTF_8)
        repeat(rounds) { state = digest.digest(state) }
        return HexFormat.of().formatHex(state)
    }
}
