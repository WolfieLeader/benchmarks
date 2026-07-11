package com.bench.shared

import com.bench.shared.web.Compute
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class ComputeTest {
    @Test
    fun `parseRounds accepts Atoi-valid values`() {
        assertEquals(1L, Compute.parseRounds("1"))
        assertEquals(1000L, Compute.parseRounds("1000"))
        // Leading + and leading zeros are decimal, per strconv.Atoi.
        assertEquals(5L, Compute.parseRounds("+5"))
        assertEquals(7L, Compute.parseRounds("007"))
        // 2^53+1 is a valid i64; the caller clamps it to the cap.
        assertEquals(9007199254740993L, Compute.parseRounds("9007199254740993"))
    }

    @Test
    fun `parseRounds rejects what Atoi rejects`() {
        assertNull(Compute.parseRounds(null))
        assertNull(Compute.parseRounds(""))
        assertNull(Compute.parseRounds("abc"))
        assertNull(Compute.parseRounds("1.5"))
        // Digit-group underscores are source-literal syntax, not input.
        assertNull(Compute.parseRounds("1_000"))
        // No trimming: surrounding whitespace fails.
        assertNull(Compute.parseRounds(" 5"))
        assertNull(Compute.parseRounds("5 "))
        // Unicode digits are not ASCII digits (Long.parseLong would accept this).
        assertNull(Compute.parseRounds("٥")) // Arabic-Indic five
        // Above i64 max (below u64 max): overflow is a parse failure, not a clamp.
        assertNull(Compute.parseRounds("9300000000000000000"))
    }

    @Test
    fun `clampRounds caps at the canon max`() {
        assertEquals(1, Compute.clampRounds(1L))
        assertEquals(1_000_000, Compute.clampRounds(5_000_000L))
        assertEquals(1_000_000, Compute.clampRounds(9007199254740993L))
    }
}
