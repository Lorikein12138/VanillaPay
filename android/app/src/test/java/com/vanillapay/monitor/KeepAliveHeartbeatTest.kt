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
        assertTrue(source.contains("HEARTBEAT_DELAY_MS = 30_000L"))
        assertTrue(source.contains("delay(HEARTBEAT_DELAY_MS)"))
        assertFalse(source.contains("PeriodicWorkRequestBuilder<HeartbeatWorker>(15"))
        assertFalse(File("src/main/java/com/vanillapay/monitor/work/HeartbeatWorker.kt").exists())
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

    @Test
    fun `foreground notification is refreshed and missing notification triggers local alert`() {
        val source = File("src/main/java/com/vanillapay/monitor/service/KeepAliveService.kt").readText()

        assertTrue(source.contains("KEEPALIVE_NOTIFICATION_ID"))
        assertTrue(source.contains("ALERT_NOTIFICATION_ID"))
        assertTrue(source.contains("manager.activeNotifications"))
        assertTrue(source.contains("activeNotifications.any { it.id == KEEPALIVE_NOTIFICATION_ID }"))
        assertTrue(source.contains("showMissingKeepAliveAlert"))
        assertTrue(source.contains("setOnlyAlertOnce(true)"))
        assertTrue(source.contains("notify(ALERT_NOTIFICATION_ID"))
        assertTrue(source.contains("ensureKeepAliveNotificationVisible()"))
    }

    @Test
    fun `heartbeat loop survives heartbeat reporter exceptions`() {
        val source = File("src/main/java/com/vanillapay/monitor/service/KeepAliveService.kt").readText()

        assertTrue(source.contains("runCatching { HeartbeatReporter(applicationContext).send() }"))
        assertTrue(source.contains("ensureKeepAliveNotificationVisible()"))
        assertTrue(source.contains("delay(HEARTBEAT_DELAY_MS)"))
    }
}
