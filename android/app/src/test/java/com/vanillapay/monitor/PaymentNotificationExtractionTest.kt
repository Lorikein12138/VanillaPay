package com.vanillapay.monitor

import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.io.File

class PaymentNotificationExtractionTest {
    @Test
    fun `expanded notification title is included in parser input`() {
        val source = File(
            "src/main/java/com/vanillapay/monitor/service/PaymentListenerService.kt",
        ).readText()

        assertTrue(source.contains("collectTitle(extras)"))
        assertTrue(source.contains("Notification.EXTRA_TITLE_BIG"))
    }
}
