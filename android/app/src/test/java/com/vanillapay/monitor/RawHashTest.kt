package com.vanillapay.monitor

import com.vanillapay.monitor.util.RawHash
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Test

class RawHashTest {
    @Test
    fun sameInputSameHashDifferentInputDifferent() {
        val a = RawHash.of("com.tencent.mm", "收款10.00元", 1000L)
        val b = RawHash.of("com.tencent.mm", "收款10.00元", 1000L)
        val c = RawHash.of("com.tencent.mm", "收款10.01元", 1000L)
        assertEquals(a, b)
        assertNotEquals(a, c)
        assertEquals(64, a.length)
    }
}
