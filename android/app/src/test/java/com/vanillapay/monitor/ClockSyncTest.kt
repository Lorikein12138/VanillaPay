package com.vanillapay.monitor

import com.vanillapay.monitor.util.ClockSync
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class ClockSyncTest {
    @Test
    fun appliesOffsetFromServer() {
        val clock = ClockSync()
        clock.sync(serverTime = 1300, localTime = 1000)
        assertEquals(1300, clock.now(localTime = 1000))
        assertEquals(1305, clock.now(localTime = 1005))
    }

    @Test
    fun defaultsToLocalBeforeSync() {
        assertEquals(1000, ClockSync().now(localTime = 1000))
    }
}
