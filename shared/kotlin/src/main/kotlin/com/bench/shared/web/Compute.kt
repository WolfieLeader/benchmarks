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
    // Go strconv.Atoi accept set: an optional leading sign then ASCII digits only.
    // The explicit [0-9] class (not \d) guards against Java's Long.parseLong, which
    // accepts Unicode digits like the Arabic-Indic "٥" via Character.digit — the
    // canon rejects those. No trimming, so surrounding whitespace also fails here.
    private val atoiPattern = Regex("[+-]?[0-9]+")

    /**
     * Parse the `/compute` `n` query value with the canon Go `strconv.Atoi`
     * semantics (signed 64-bit, base-10, ASCII): a leading `+` and leading zeros
     * are accepted; underscores, whitespace, Unicode digits, and values outside
     * the i64 range (`toLongOrNull` returns null on overflow) are not. Callers
     * still gate on `>= 1` and clamp via [clampRounds].
     */
    fun parseRounds(raw: String?): Long? {
        if (raw == null || !atoiPattern.matches(raw)) return null
        return raw.toLongOrNull()
    }

    /** Clamp `n` to the cap: values above [Consts.COMPUTE_MAX_ROUNDS] are rounded down. */
    fun clampRounds(n: Long): Int = minOf(n, Consts.COMPUTE_MAX_ROUNDS).toInt()

    fun sha256Chain(rounds: Int): String {
        val digest = MessageDigest.getInstance("SHA-256")
        var state = Consts.COMPUTE_SEED.toByteArray(Charsets.UTF_8)
        repeat(rounds) { state = digest.digest(state) }
        return HexFormat.of().formatHex(state)
    }
}
