package com.vanillapay.monitor

import android.app.Application
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.service.notification.NotificationListenerService
import androidx.core.content.ContextCompat
import androidx.room.Room
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import com.vanillapay.monitor.parse.Channel
import com.vanillapay.monitor.parse.PaymentNotice
import com.vanillapay.monitor.parse.isDiagnostic
import com.vanillapay.monitor.parse.parseReceipt
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.channels.Channel as Signal
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import java.util.concurrent.TimeUnit

data class MonitorState(
    val loaded: Boolean = false,
    val session: Credential? = null,
    val online: Boolean = false,
    val banned: Boolean = false,
    val lastHeartbeat: Long? = null,
)

class MonitorApp : Application() {
    val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    val api = DeviceApi()
    val database by lazy { Room.databaseBuilder(this, MonitorDatabase::class.java, "monitor.db").build() }
    private val vault by lazy { CredentialVault(this) }
    private val sessionLock = Mutex()
    private val mutableState = MutableStateFlow(MonitorState())
    val state = mutableState.asStateFlow()
    val uploads = Signal<Unit>(Signal.CONFLATED)
    // Read and written on the main thread alongside the listener callbacks.
    var listenerConnected = false

    override fun onCreate() {
        super.onCreate()
        scope.launch {
            val session = withContext(Dispatchers.IO) { vault.load() }
            mutableState.value = MonitorState(loaded = true, session = session)
        }
        val workManager = WorkManager.getInstance(this)
        workManager.enqueueUniquePeriodicWork("monitor-history-retention", ExistingPeriodicWorkPolicy.KEEP,
            PeriodicWorkRequestBuilder<RetentionWorker>(1, TimeUnit.DAYS).build())
        workManager.enqueueUniquePeriodicWork("monitor-revive", ExistingPeriodicWorkPolicy.KEEP,
            PeriodicWorkRequestBuilder<ReviveWorker>(15, TimeUnit.MINUTES).build())
    }

    suspend fun bind(base: String, code: String, name: String) = sessionLock.withLock {
        check(state.first { it.loaded }.session == null)
        val session = api.bind(base, code, name)
        withContext(Dispatchers.IO) { vault.save(session) }
        mutableState.value = MonitorState(loaded = true, session = session)
        pruneLocked()
    }

    suspend fun startMonitoring() = sessionLock.withLock {
        val session = mutableState.value.session ?: return@withLock
        val started = session.copy(started = true)
        withContext(Dispatchers.IO) { vault.save(started) }
        mutableState.value = mutableState.value.copy(session = started)
        launchService()
    }

    fun launchService() {
        ContextCompat.startForegroundService(this, Intent(this, MonitorService::class.java))
    }

    suspend fun revive(rebindOnly: Boolean = false) = withContext(Dispatchers.Main.immediate) {
        state.first { it.loaded }
        val actions = reviveActions(state.value, notificationAccess(this@MonitorApp), listenerConnected)
        // Background foreground-service starts can be refused by Android or vendor firmware.
        if (!rebindOnly && actions.launchService) runCatching { launchService() }
        if (actions.requestRebind) runCatching {
            NotificationListenerService.requestRebind(ComponentName(this@MonitorApp, PaymentNotificationListener::class.java))
        }
    }

    suspend fun heartbeat(deviceId: String, answer: Heartbeat) = sessionLock.withLock {
        val current = mutableState.value
        val session = current.session?.takeIf { it.deviceId == deviceId } ?: return@withLock
        val updated = session.copy(offsetMillis = answer.offsetMillis, interval = answer.interval)
        withContext(Dispatchers.IO) { vault.save(updated) }
        mutableState.value = current.copy(session = updated, online = answer.online, banned = answer.banned,
            lastHeartbeat = System.currentTimeMillis())
    }

    fun offline(deviceId: String) {
        val current = mutableState.value
        if (current.session?.deviceId == deviceId) mutableState.value = current.copy(online = false)
    }

    suspend fun forget(deviceId: String) = sessionLock.withLock {
        if (mutableState.value.session?.deviceId != deviceId) return@withLock
        withContext(Dispatchers.IO) { vault.clear() }
        mutableState.value = MonitorState(loaded = true)
        MonitorNotifications.stopped(this)
        pruneLocked()
    }

    suspend fun capture(notice: PaymentNotice, scopeAtDelivery: String?) = sessionLock.withLock {
        val channel = Channel.entries.firstOrNull { it.packageName == notice.packageName } ?: return@withLock
        val parsed = parseReceipt(notice)
        val diagnostic = channel.wire.takeIf { isDiagnostic(notice) }
        if (parsed == null && diagnostic == null) return@withLock
        val session = mutableState.value.session?.takeIf { it.deviceId == scopeAtDelivery }
        if (database.capture(notice, parsed, diagnostic, session, System.currentTimeMillis())) {
            uploads.trySend(Unit)
        }
    }

    suspend fun prune() {
        state.first { it.loaded }
        sessionLock.withLock { pruneLocked() }
    }

    private suspend fun pruneLocked() {
        val scope = state.value.session?.deviceId.orEmpty()
        val cutoff = System.currentTimeMillis() - WEEK
        database.dao().apply {
            pruneDiagnostics(cutoff)
            pruneStamps(cutoff, scope)
            pruneReceipts(cutoff, scope)
        }
    }
}

internal data class ReviveActions(val launchService: Boolean, val requestRebind: Boolean)

internal fun reviveActions(state: MonitorState, hasNotificationAccess: Boolean, listenerConnected: Boolean): ReviveActions {
    val started = state.loaded && state.session?.started == true
    return ReviveActions(started, started && hasNotificationAccess && !listenerConnected)
}

class ReviveWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        (applicationContext as MonitorApp).revive()
        return Result.success()
    }
}

class RetentionWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        (applicationContext as MonitorApp).prune()
        return Result.success()
    }
}
