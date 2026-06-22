package com.vanillapay.monitor

import com.vanillapay.monitor.net.Backoff
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class BackoffTest {
    @Test
    fun delaysFollowSchedule() {
        assertEquals(5L, Backoff.delaySeconds(1))
        assertEquals(15L, Backoff.delaySeconds(2))
        assertEquals(300L, Backoff.delaySeconds(6))
        assertEquals(300L, Backoff.delaySeconds(99))
    }
}
