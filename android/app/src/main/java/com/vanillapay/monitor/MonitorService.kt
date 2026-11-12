package com.vanillapay.monitor

import android.Manifest
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull
import org.json.JSONException
import java.io.IOException
import java.math.BigDecimal
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import kotlin.coroutines.coroutineContext

fun notificationAccess(context: Context) =
    context.packageName in NotificationManagerCompat.getEnabledListenerPackages(context)

internal fun todayStart(): Long = LocalDate.now().atStartOfDay(ZoneId.systemDefault()).toInstant().toEpochMilli()

internal fun yuan(cents: Long) = "¥${BigDecimal.valueOf(cents, 2).toPlainString()}"

internal fun localTime(millis: Long, format: String): String = DateTimeFormatter.ofPattern(format)
    .withZone(ZoneId.systemDefault()).format(Instant.ofEpochMilli(millis))

class MonitorService : Service() {
    private val tasks = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private val app get() = application as MonitorApp
    private val dao get() = app.database.dao()

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onCreate() {
        super.onCreate()
        val initial = MonitorNotifications.offline(0)
        val type = if (Build.VERSION.SDK_INT >= 34) ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE else 0
        ServiceCompat.startForeground(this, MonitorNotifications.ID, MonitorNotifications.build(this, initial), type)
        tasks.launch {
            app.state.map { state -> state.loaded to state.session?.takeIf { it.started }?.deviceId }
                .distinctUntilChanged().collectLatest { (loaded, id) ->
                    if (!loaded) return@collectLatest
                    if (id == null) {
                        stopForeground(STOP_FOREGROUND_REMOVE)
                        if (app.state.value.session == null) MonitorNotifications.stopped(this@MonitorService)
                        stopSelf()
                        return@collectLatest
                    }
                    kotlinx.coroutines.coroutineScope {
                        val heartbeatWake = Channel<Unit>(Channel.CONFLATED)
                        launch { heartbeat(id, heartbeatWake) }
                        launch { upload(id, heartbeatWake) }
                        launch { notifications(id, initial) }
                    }
                }
        }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int) = START_STICKY

    override fun onDestroy() {
        tasks.cancel()
        super.onDestroy()
    }

    private fun session(id: String) = app.state.value.session?.takeIf { it.deviceId == id }

    private suspend fun heartbeat(id: String, wake: Channel<Unit>) {
        while (coroutineContext.isActive) {
            val session = session(id) ?: return
            if (!notificationAccess(this)) {
                app.offline(id)
                delay(3_000)
                continue
            }
            try {
                app.heartbeat(id, app.api.heartbeat(session))
            } catch (e: ApiFailure) {
                if (e.unauthorized) { app.forget(id); return }
                app.offline(id)
            } catch (_: IOException) {
                app.offline(id)
            } catch (_: JSONException) {
                app.offline(id)
            }
            val interval = session(id)?.interval ?: return
            withTimeoutOrNull(interval * 1_000L) { wake.receive() }
        }
    }

    private suspend fun upload(id: String, wake: Channel<Unit>) {
        while (coroutineContext.isActive) {
            val session = session(id) ?: return
            val now = System.currentTimeMillis()
            val receipt = dao.next(id, now)
            if (receipt == null) {
                val next = dao.nextRetry(id)
                if (next == null) app.uploads.receive()
                else withTimeoutOrNull((next - now).coerceIn(1, 300_000)) { app.uploads.receive() }
                continue
            }
            try {
                val result = app.api.report(session, receipt.payload)
                dao.complete(receipt.eventId, if (result.matched) MATCHED else UNMATCHED,
                    result.tradeNo ?: result.reason, System.currentTimeMillis())
                if (!app.state.value.online) wake.trySend(Unit)
            } catch (e: ApiFailure) {
                if (e.unauthorized) { app.forget(id); return }
                if (e.retryable) retry(receipt) else dao.complete(receipt.eventId, FAILED, e.code.ifBlank { "HTTP ${e.status}" }, System.currentTimeMillis())
            } catch (_: IOException) {
                retry(receipt)
            } catch (_: JSONException) {
                retry(receipt)
            }
        }
    }

    private suspend fun retry(receipt: Receipt) {
        dao.retry(receipt.eventId, (receipt.attempts + 1).coerceAtMost(30), System.currentTimeMillis() + retryDelay(receipt.attempts))
    }

    private suspend fun notifications(id: String, initial: NoticeCopy) {
        var previous = initial
        while (coroutineContext.isActive && session(id) != null) {
            val state = app.state.value
            val copy = when {
                !notificationAccess(this) -> MonitorNotifications.paused
                !state.online -> MonitorNotifications.offline(dao.pending(id).first())
                else -> {
                    val last = dao.recent(id).first().firstOrNull { it.status == MATCHED || it.status == UNMATCHED }
                    MonitorNotifications.online(dao.reported(id, todayStart()).first(), last)
                }
            }
            if (copy != previous && session(id) != null) {
                MonitorNotifications.post(this, copy)
                previous = copy
            }
            delay(2_000)
        }
    }
}

data class NoticeCopy(val status: String, val title: String, val body: String)

object MonitorNotifications {
    const val ID = 100
    private const val CHANNEL = "monitor-status"
    val paused = NoticeCopy("已暂停", "读不到收款通知了", "通知读取权限被关闭，轻点重新开启")
    private val stopped = NoticeCopy("已停止", "这台手机已解绑", "不再上报到账，轻点重新绑定")

    fun online(count: Int, last: Receipt?) = NoticeCopy("在线", "收款监听中", buildString {
        append("今日已上报 $count 笔")
        if (last != null) append("，最近一笔 ${yuan(last.cents)}（${localTime(last.occurredAt, "HH:mm")}）")
    })

    fun offline(pending: Int) = NoticeCopy("离线", "网络已断开，监听没有中断", "${pending} 笔到账暂存在本机，联网后自动补传")

    fun stopped(context: Context) = post(context, stopped)

    fun post(context: Context, copy: NoticeCopy) {
        if (Build.VERSION.SDK_INT >= 33 &&
            ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) return
        context.getSystemService(NotificationManager::class.java).notify(ID, build(context, copy))
    }

    fun build(context: Context, copy: NoticeCopy): Notification {
        val channel = NotificationChannel(CHANNEL, "收款监听", NotificationManager.IMPORTANCE_DEFAULT).apply {
            setSound(null, null)
            enableVibration(false)
            setShowBadge(false)
            enableLights(false)
            lockscreenVisibility = Notification.VISIBILITY_PRIVATE
        }
        val manager = context.getSystemService(NotificationManager::class.java)
        manager.createNotificationChannel(channel)
        manager.deleteNotificationChannel("payment-monitor")
        val intent = Intent(context, MainActivity::class.java)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP or Intent.FLAG_ACTIVITY_SINGLE_TOP)
        val open = PendingIntent.getActivity(context, 0, intent, PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
        return NotificationCompat.Builder(context, CHANNEL)
            .setSmallIcon(R.drawable.ic_notification)
            .setColor(ContextCompat.getColor(context, R.color.notification_accent))
            .setColorized(true)
            .setContentTitle(copy.title)
            .setContentText(copy.body)
            .setSubText(copy.status)
            .setStyle(NotificationCompat.BigTextStyle().bigText(copy.body))
            .setContentIntent(open)
            .setPriority(NotificationCompat.PRIORITY_MAX)
            .setCategory(NotificationCompat.CATEGORY_SERVICE)
            .setForegroundServiceBehavior(NotificationCompat.FOREGROUND_SERVICE_IMMEDIATE)
            .setOngoing(true)
            .setSilent(true)
            .setOnlyAlertOnce(true)
            .setVisibility(NotificationCompat.VISIBILITY_PRIVATE)
            .build()
    }
}
