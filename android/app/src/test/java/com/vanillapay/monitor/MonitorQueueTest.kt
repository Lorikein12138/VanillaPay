package com.vanillapay.monitor

import android.app.Application
import androidx.room.Room
import com.vanillapay.monitor.parse.Channel
import com.vanillapay.monitor.parse.PaymentNotice
import com.vanillapay.monitor.parse.parseReceipt
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import java.time.Instant

@RunWith(RobolectricTestRunner::class)
@Config(application = Application::class, sdk = [35])
class MonitorQueueTest {
    private val session = Credential("https://example.test/", "device-a", "fixture", "Phone", 0, 30, 3500, started = true)
    private val initial = PaymentNotice("provider", Channel.WeChat.packageName, 1_000, 1_000, "微信支付", "微信支付收款0.01元")

    @Test fun duplicateAndMergedUpdateQueueOnceButSeparateSameAmountQueuesAgain() = runBlocking {
        withDatabase { db ->
            assertTrue(capture(db, initial))
            assertFalse(capture(db, initial))
            val merged = initial.copy(postTime = 2_000, title = "[2条]微信支付", body = "微信支付: 收款0.01元")
            assertFalse(capture(db, merged))
            assertTrue(capture(db, merged.copy(postTime = 4_000, notificationTime = 4_000, title = "[3条]微信支付")))
            assertEquals(2, db.dao().pending(session.deviceId).first())
        }
    }

    @Test fun storesTheCorrectedTimeAndExactPayloadAcrossRetries() = runBlocking {
        withDatabase { db ->
            assertTrue(capture(db, initial))
            val pending = db.dao().next(session.deviceId, 2_000)!!
            val json = JSONObject(pending.payload)
            assertEquals(Instant.ofEpochMilli(4_500), Instant.parse(json.getString("occurred_at")))
            assertEquals(pending.eventId, json.getString("event_id"))
            db.dao().retry(pending.eventId, 1, 5_000)
            assertNull(db.dao().next(session.deviceId, 4_999))
            val retry = db.dao().next(session.deviceId, 5_000)!!
            assertArrayEquals(pending.payload.toByteArray(), retry.payload.toByteArray())
            assertNull(db.dao().next("another-device", 5_000))
            db.dao().complete(pending.eventId, FAILED, "EVENT_CONFLICT", 5_000)
            assertNull(db.dao().next(session.deviceId, 6_000))
            assertEquals(pending.payload, db.dao().recent(session.deviceId).first().single().payload)
        }
    }

    @Test fun diagnosticsNeverEnqueueWithoutStartedSessionAndRetainOnlyRecentRecords() = runBlocking {
        withDatabase { db ->
            repeat(55) { index ->
                val notice = initial.copy(key = "notification-$index", postTime = 1_000L + index)
                assertFalse(db.capture(notice, parseReceipt(notice), "wxpay", null, 5_000))
            }
            assertEquals(50, db.dao().diagnostics().first().size)
            assertEquals(0, db.dao().pending(session.deviceId).first())
            db.dao().pruneDiagnostics(5_000)
            assertTrue(db.dao().diagnostics().first().isEmpty())
        }
    }

    private suspend fun capture(db: MonitorDatabase, notice: PaymentNotice) =
        db.capture(notice, parseReceipt(notice), "wxpay", session, 5_000)

    private suspend fun withDatabase(block: suspend (MonitorDatabase) -> Unit) {
        val db = Room.inMemoryDatabaseBuilder(RuntimeEnvironment.getApplication(), MonitorDatabase::class.java).build()
        try { block(db) } finally { db.close() }
    }
}
