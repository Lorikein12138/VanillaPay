package com.vanillapay.monitor

import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.io.File

class KeepAliveHeartbeatTest {
    @Test
    fun `foreground service sends heartbeat every thirty seconds`() {
        val source = File("src/main/java/com/vanillapay/monitor/service/KeepAliveService.kt").readText()

        assertTrue(source.contains("HeartbeatReporter(applicationContext).send()"))
        assertTrue(source.contains("delay(30_000L)"))
        assertFalse(source.contains("PeriodicWorkRequestBuilder<HeartbeatWorker>(15"))
    }
}
