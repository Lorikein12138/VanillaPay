package com.vanillapay.monitor

import android.app.NotificationManager
import android.content.ComponentName
import android.os.Looper
import android.provider.Settings
import com.vanillapay.monitor.parse.Channel
import com.vanillapay.monitor.parse.PaymentNotice
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.*
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.Robolectric
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.Shadows.shadowOf
import org.robolectric.annotation.Config
import org.robolectric.annotation.Implementation
import org.robolectric.annotation.Implements
import org.robolectric.annotation.LooperMode
import org.robolectric.shadows.ShadowApplication
import org.robolectric.util.ReflectionHelpers
import java.util.concurrent.TimeUnit

class ReviveActionsTest {
    private val session = Credential("https://example.test/", "device-a", "fixture", "Phone", 0, 60, 0, started = true)

    @Test fun onlyStartedAuthorizedSessionsCanReviveAndOnlyDisconnectedListenersRebind() {
        assertEquals(ReviveActions(false, false), reviveActions(MonitorState(session = session), true, false))
        assertEquals(ReviveActions(false, false), reviveActions(MonitorState(loaded = true), true, false))
        val state = MonitorState(loaded = true, session = session)
        assertEquals(ReviveActions(false, false), reviveActions(state.copy(session = session.copy(started = false)), true, false))
        assertEquals(ReviveActions(false, false), reviveActions(state, false, false))
        assertEquals(ReviveActions(true, false), reviveActions(state, true, true))
        assertEquals(ReviveActions(true, true), reviveActions(state, true, false))
    }
}

@RunWith(RobolectricTestRunner::class)
@Config(application = MonitorApp::class, sdk = [35], shadows = [ShadowMonitorAppStartup::class])
@LooperMode(LooperMode.Mode.PAUSED)
class MonitorReviveTest {
    private lateinit var app: MonitorApp
    private val session = Credential("https://example.test/", "device-a", "fixture", "Phone", 0, 60, 0, started = true)

    @Before fun setUp() {
        app = RuntimeEnvironment.getApplication() as MonitorApp
    }

    @After fun tearDown() {
        app.scope.cancel()
    }

    @Test fun listenerStartsMonitorWhenCreatedConnectedAndDisconnected() {
        setNotificationAccess(true)
        setState(MonitorState(loaded = true, session = session))
        val service = Robolectric.buildService(PaymentNotificationListener::class.java).create()
        try {
            val monitor = ComponentName(app, MonitorService::class.java)
            assertEquals("create", monitor, shadowOf(app).nextStartedService?.component)
            service.get().onListenerConnected()
            assertTrue(app.listenerConnected)
            assertEquals("connect", monitor, shadowOf(app).nextStartedService?.component)
            service.get().onListenerDisconnected()
            assertFalse(app.listenerConnected)
            assertEquals("disconnect", monitor, shadowOf(app).nextStartedService?.component)
        } finally {
            service.destroy()
        }
    }

    @Test fun listenerDoesNotStartMonitorWhenPausedNotStartedOrUnbound() {
        val cases = listOf(
            Triple("paused", MonitorState(loaded = true, session = session), false),
            Triple("not started", MonitorState(loaded = true, session = session.copy(started = false)), true),
            Triple("unbound or stopped", MonitorState(loaded = true), true),
        )
        for ((label, state, hasAccess) in cases) {
            setNotificationAccess(hasAccess)
            setState(state)
            val service = Robolectric.buildService(PaymentNotificationListener::class.java).create()
            try {
                assertNull("$label on create", shadowOf(app).nextStartedService)
                service.get().onListenerConnected()
                assertNull("$label on connect", shadowOf(app).nextStartedService)
                service.get().onListenerDisconnected()
                assertNull("$label on disconnect", shadowOf(app).nextStartedService)
            } finally {
                service.destroy()
            }
        }
    }

    @Test fun listenerWaitsUntilTheStartedSessionIsLoaded() {
        setNotificationAccess(true)
        setState(MonitorState(session = session))
        val service = Robolectric.buildService(PaymentNotificationListener::class.java).create()
        try {
            assertNull("create before load", shadowOf(app).nextStartedService)
            service.get().onListenerConnected()
            assertNull("connect before load", shadowOf(app).nextStartedService)

            setState(MonitorState(loaded = true, session = session))
            shadowOf(Looper.getMainLooper()).idle()
            assertEquals(ComponentName(app, MonitorService::class.java), shadowOf(app).nextStartedService?.component)
        } finally {
            service.destroy()
        }
    }

    @Test fun foregroundNotificationShowsImmediatelyWithLowImportance() {
        val notice = MonitorNotifications.build(app, MonitorNotifications.offline(0))
        // Read the platform's display policy from the built notification (hidden from the SDK stubs).
        assertTrue(ReflectionHelpers.callInstanceMethod<Boolean>(notice, "shouldShowForegroundImmediately"))
        val channel = app.getSystemService(NotificationManager::class.java).getNotificationChannel(notice.channelId)
        assertEquals("payment-monitor", channel.id)
        assertEquals(NotificationManager.IMPORTANCE_LOW, channel.importance)
    }

    @Test fun successfulUploadWhileOfflineWakesHeartbeatBeforeItsNormalInterval() {
        MockWebServer().use { server ->
            server.start()
            setState(MonitorState(loaded = true, session = session.copy(base = server.url("/").toString()), online = true))
            setNotificationAccess(true)
            server.enqueue(MockResponse().setResponseCode(503))
            server.enqueue(MockResponse().setBody("""{"data":{"match_state":"matched","trade_no":"order-1"}}"""))
            server.enqueue(MockResponse().setResponseCode(503))
            val service = Robolectric.buildService(MonitorService::class.java).create()
            try {
                assertEquals("/api/v1/device/heartbeat", nextRequestPath(server))
                waitFor { !app.state.value.online }

                runBlocking {
                    val now = System.currentTimeMillis()
                    app.capture(PaymentNotice("provider", Channel.WeChat.packageName, now, now,
                        "微信支付", "微信支付收款0.01元"), session.deviceId)
                }
                assertEquals("/api/v1/device/receipts", nextRequestPath(server))
                // The paused main looper never advances to the 60-second heartbeat timer.
                assertEquals("/api/v1/device/heartbeat", nextRequestPath(server))
                assertFalse(app.state.value.online)
                assertEquals(MATCHED, runBlocking { app.database.dao().recent(session.deviceId).first().single().status })
            } finally {
                service.destroy()
                app.database.close()
            }
        }
    }

    private fun setState(state: MonitorState) {
        ReflectionHelpers.getField<MutableStateFlow<MonitorState>>(app, "mutableState").value = state
    }

    private fun setNotificationAccess(enabled: Boolean) {
        val listener = ComponentName(app, PaymentNotificationListener::class.java).flattenToString()
        Settings.Secure.putString(app.contentResolver, "enabled_notification_listeners", if (enabled) listener else "")
    }

    private fun nextRequestPath(server: MockWebServer): String? {
        var path: String? = null
        waitFor {
            path = server.takeRequest(20, TimeUnit.MILLISECONDS)?.path
            path != null
        }
        return path
    }

    private fun waitFor(condition: () -> Boolean) {
        val deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(5)
        do {
            shadowOf(Looper.getMainLooper()).idle()
            if (condition()) return
            Thread.yield()
        } while (System.nanoTime() < deadline)
        fail("Condition did not become true before timeout")
    }
}

@Implements(MonitorApp::class)
class ShadowMonitorAppStartup : ShadowApplication() {
    // The fixture supplies credential state without opening the vault or scheduling retention work.
    @Implementation fun onCreate() = Unit
}
