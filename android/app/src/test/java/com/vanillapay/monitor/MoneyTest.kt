package com.vanillapay.monitor

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class MoneyTest {
    @Test
    fun toCents() {
        assertEquals(1000L, Money.toCents("10.00"))
        assertEquals(999L, Money.toCents("9.99"))
        assertEquals(8850L, Money.toCents("88.5"))
    }

    @Test
    fun format() {
        assertEquals("10.03", Money.format(1003))
        assertEquals("0.01", Money.format(1))
    }
}
