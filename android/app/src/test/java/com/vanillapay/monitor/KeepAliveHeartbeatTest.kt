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

    @Test
    fun `foreground notification uses a vector status icon without a large badge icon`() {
        val source = File("src/main/java/com/vanillapay/monitor/service/KeepAliveService.kt").readText()

        assertTrue(source.contains(".setSmallIcon(R.drawable.ic_notification_vanillapay)"))
        assertTrue(source.contains(".setBadgeIconType(NotificationCompat.BADGE_ICON_NONE)"))
        assertFalse(source.contains(".setLargeIcon("))
        assertFalse(source.contains("R.drawable.ic_stat_monitor"))
    }

    @Test
    fun `foreground service start mirrors gkd service compat pattern`() {
        val source = File("src/main/java/com/vanillapay/monitor/service/KeepAliveService.kt").readText()

        assertTrue(source.contains("ServiceCompat.startForeground("))
        assertTrue(source.contains("ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC"))
        assertFalse(source.contains("startForeground(1, notification)"))
    }
}
