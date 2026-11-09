package com.vanillapay.monitor

import android.content.ComponentName
import android.os.Looper
import android.provider.Settings
import androidx.work.ListenableWorker.Result
import androidx.work.WorkManager
import androidx.work.testing.TestListenableWorkerBuilder
import androidx.work.testing.WorkManagerTestInitHelper
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
import org.robolectric.annotation.RealObject
import org.robolectric.shadow.api.Shadow
import org.robolectric.shadows.ShadowApplication
import org.robolectric.util.ReflectionHelpers
import java.util.concurrent.TimeUnit

class ReviveActionsTest {
    private val session = Credential("https://example.test/", "device-a", "fixture", "Phone", 0, 60, 0, started = true)

    @Test fun onlyStartedSessionsCanReviveAndOnlyDisconnectedAuthorizedListenersRebind() {
        assertEquals(ReviveActions(false, false), reviveActions(MonitorState(session = session), true, false))
        assertEquals(ReviveActions(false, false), reviveActions(MonitorState(loaded = true), true, false))
        val state = MonitorState(loaded = true, session = session)
        assertEquals(ReviveActions(false, false), reviveActions(state.copy(session = session.copy(started = false)), true, false))
        assertEquals(ReviveActions(true, false), reviveActions(state, false, false))
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
        waitFor { app.state.value.loaded }
    }

    @After fun tearDown() {
        app.scope.cancel()
    }

    @Test fun applicationKeepsOnePeriodicWorkWithTheSameId() {
        val manager = WorkManager.getInstance(app)
        val first = manager.getWorkInfosForUniqueWork("monitor-revive").get(5, TimeUnit.SECONDS).single()
        assertTrue(first.tags.contains(ReviveWorker::class.java.name))
        assertEquals(TimeUnit.MINUTES.toMillis(15), first.periodicityInfo?.repeatIntervalMillis)

        Shadow.directlyOn<Any, MonitorApp>(app, MonitorApp::class.java, "onCreate")
        assertEquals(first.id, manager.getWorkInfosForUniqueWork("monitor-revive").get(5, TimeUnit.SECONDS).single().id)
    }

    @Test fun workerAttemptsToStartTheServiceForAStartedSession() = runBlocking {
        setSession(session)
        assertEquals(Result.success(), TestListenableWorkerBuilder<ReviveWorker>(app).build().doWork())
        assertEquals(ComponentName(app, MonitorService::class.java), shadowOf(app).nextStartedService?.component)
    }

    @Test fun workerLeavesTheServiceStoppedWhenMonitoringHasNotStarted() = runBlocking {
        setSession(session.copy(started = false))
        assertEquals(Result.success(), TestListenableWorkerBuilder<ReviveWorker>(app).build().doWork())
        assertNull(shadowOf(app).nextStartedService)
    }

    @Test fun successfulUploadWhileOfflineWakesHeartbeatBeforeItsNormalInterval() {
        MockWebServer().use { server ->
            server.start()
            setSession(session.copy(base = server.url("/").toString()), online = true)
            Settings.Secure.putString(app.contentResolver, "enabled_notification_listeners",
                ComponentName(app, PaymentNotificationListener::class.java).flattenToString())
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

    private fun setSession(session: Credential, online: Boolean = false) {
        // Seed an already-loaded session without depending on the device's Android Keystore.
        ReflectionHelpers.getField<MutableStateFlow<MonitorState>>(app, "mutableState").value =
            MonitorState(loaded = true, session = session, online = online)
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
    @RealObject private lateinit var app: MonitorApp

    @Implementation fun onCreate() {
        // Robolectric does not run the AndroidX Startup provider before Application.onCreate.
        WorkManagerTestInitHelper.initializeTestWorkManager(app)
        Shadow.directlyOn<Any, MonitorApp>(app, MonitorApp::class.java, "onCreate")
    }
}
