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
import java.util.UUID

@RunWith(RobolectricTestRunner::class)
@Config(application = Application::class, sdk = [35])
class MonitorQueueTest {
    private val session = Credential("https://example.test/", "device-a", "fixture", "Phone", 0, 30, 3500, started = true)
    private val initial = PaymentNotice("provider", Channel.WeChat.packageName, 1_000, 1_000, "微信支付", "微信支付收款0.01元")

    @Test fun alipayReceiptTitleEnqueuesScreenshotAmount() = runBlocking {
        withDatabase { db ->
            val notice = alipayTitleNotice
            val dao = db.dao()
            assertTrue("截图中的支付宝收款通知应入队", captureAlipay(db, notice))
            assertEquals(1, dao.pending(session.deviceId).first())
            val next = dao.next(session.deviceId, 5_000)!!
            val payload = JSONObject(next.payload)
            assertEquals(QUEUED, next.status)
            assertEquals(5000L, payload.getLong("amount_cents"))
            assertEquals("alipay", payload.getString("channel"))
            assertEquals("${notice.title}\n${notice.body}", payload.getString("raw_text"))
            val eventId = payload.getString("event_id")
            assertEquals(next.eventId, eventId)
            assertEquals(eventId, UUID.fromString(eventId).toString())
            assertNull(next.providerTradeNo)
            assertFalse(payload.has("provider_trade_no"))

            assertFalse(captureAlipay(db, notice))
            val update = notice.copy(
                postTime = 2_000, title = "[2条]你已成功收款50.0元(常客)", body = "[2条]${notice.body}",
            )
            assertFalse(captureAlipay(db, update))
            assertEquals(1, dao.pending(session.deviceId).first())

            val second = notice.copy(postTime = 3_000, notificationTime = 3_000)
            assertTrue(captureAlipay(db, second))
            assertEquals(2, dao.pending(session.deviceId).first())
            assertTrue(captureAlipay(db, second.copy(key = "another-alipay-provider")))
            assertEquals(3, dao.pending(session.deviceId).first())
            assertEquals(3, dao.recent(session.deviceId).first().map { it.eventId }.toSet().size)
        }
    }

    @Test fun alipayTitleAmountChangeCanEnqueueWithTheSameNotificationTime() = runBlocking {
        withDatabase { db ->
            val notice = alipayTitleNotice
            assertTrue(captureAlipay(db, notice))
            assertTrue(captureAlipay(db, notice.copy(postTime = 2_000, title = "你已成功收款51.00元")))
            assertEquals(listOf(5100L, 5000L), db.dao().recent(session.deviceId).first().map { it.cents })
        }
    }

    @Test fun alipayTitleCountUpdateWithoutNotificationTimeQueuesOnlyOnce() = runBlocking {
        withDatabase { db ->
            val notice = alipayTitleNotice.copy(notificationTime = 0)
            assertTrue(captureAlipay(db, notice))
            assertFalse(captureAlipay(db, notice.copy(postTime = 2_000, title = "[2条]${notice.title}")))
            assertEquals(1, db.dao().pending(session.deviceId).first())
        }
    }

    @Test fun alipayTitleWithoutStartedSessionAndUnrecognizedNoticeOnlySaveDiagnostics() = runBlocking {
        withDatabase { db ->
            val notice = alipayTitleNotice
            val invalid = notice.copy(postTime = 2_000, body = "已转入余额")
            assertFalse(db.capture(notice, parseReceipt(notice), "alipay", session.copy(started = false), 5_000))
            assertFalse(captureAlipay(db, invalid))
            val diagnostics = db.dao().diagnostics().first()
            assertEquals(listOf(2_000L, 1_000L), diagnostics.map { it.postTime })
            assertEquals(listOf(null, 5000L), diagnostics.map { it.cents })
            assertEquals(0, db.dao().pending(session.deviceId).first())
        }
    }

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

    private suspend fun captureAlipay(db: MonitorDatabase, notice: PaymentNotice) =
        db.capture(notice, parseReceipt(notice), "alipay", session, 5_000)

    private suspend fun withDatabase(block: suspend (MonitorDatabase) -> Unit) {
        val db = Room.inMemoryDatabaseBuilder(RuntimeEnvironment.getApplication(), MonitorDatabase::class.java).build()
        try { block(db) } finally { db.close() }
    }
}
