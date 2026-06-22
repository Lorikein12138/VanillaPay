package com.vanillapay.monitor.service

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat
import com.vanillapay.monitor.R
import com.vanillapay.monitor.ui.MainActivity
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import com.vanillapay.monitor.net.HeartbeatReporter
import com.vanillapay.monitor.work.RetryWorker
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import java.util.concurrent.TimeUnit

class KeepAliveService : Service() {
    private var heartbeatJob: Job? = null
    private var lastMissingKeepAliveAlertAt: Long = 0L

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onCreate() {
        super.onCreate()
        createNotificationChannels()
        startForegroundWithKeepAliveNotification()
        scheduleWorkers()
        startHeartbeatLoop()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int = START_STICKY

    private fun createNotificationChannels() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val manager = getSystemService(NotificationManager::class.java)
            manager.createNotificationChannel(
                NotificationChannel(KEEPALIVE_CHANNEL_ID, "监听服务", NotificationManager.IMPORTANCE_LOW),
            )
            manager.createNotificationChannel(
                NotificationChannel(ALERT_CHANNEL_ID, "监听异常", NotificationManager.IMPORTANCE_HIGH),
            )
        }
    }

    private fun startForegroundWithKeepAliveNotification() {
        ServiceCompat.startForeground(
            this,
            KEEPALIVE_NOTIFICATION_ID,
            buildKeepAliveNotification(),
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                ServiceInfo.FOREGROUND_SERVICE_TYPE_MANIFEST
            } else {
                0
            },
        )
    }

    private fun buildKeepAliveNotification(): Notification {
        val contentIntent = PendingIntent.getActivity(
            this,
            0,
            Intent(this, MainActivity::class.java)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP),
            PendingIntent.FLAG_IMMUTABLE,
        )
        return NotificationCompat.Builder(this, KEEPALIVE_CHANNEL_ID)
            .setContentTitle("VanillaPay 监听运行中")
            .setContentText("正在监听微信 / 支付宝到账通知")
            .setSmallIcon(R.drawable.ic_notification_vanillapay)
            .setBadgeIconType(NotificationCompat.BADGE_ICON_NONE)
            .setColor(ContextCompat.getColor(this, R.color.brand_primary))
            .setContentIntent(contentIntent)
            .setForegroundServiceBehavior(NotificationCompat.FOREGROUND_SERVICE_IMMEDIATE)
            .setOngoing(true)
            .build()
    }

    private fun scheduleWorkers() {
        val workManager = WorkManager.getInstance(this)
        workManager.enqueueUniquePeriodicWork(
            "retry",
            ExistingPeriodicWorkPolicy.KEEP,
            PeriodicWorkRequestBuilder<RetryWorker>(15, TimeUnit.MINUTES).build(),
        )
    }

    private fun startHeartbeatLoop() {
        if (heartbeatJob?.isActive == true) return

        heartbeatJob = CoroutineScope(Dispatchers.IO).launch {
            while (isActive) {
                runCatching { HeartbeatReporter(applicationContext).send() }
                ensureKeepAliveNotificationVisible()
                delay(HEARTBEAT_DELAY_MS)
            }
        }
    }

    private fun ensureKeepAliveNotificationVisible() {
        val manager = getSystemService(NotificationManager::class.java)
        val keepAliveVisible =
            manager.activeNotifications.any { it.id == KEEPALIVE_NOTIFICATION_ID }
        manager.notify(KEEPALIVE_NOTIFICATION_ID, buildKeepAliveNotification())
        if (!keepAliveVisible) {
            showMissingKeepAliveAlert(manager)
        }
    }

    private fun showMissingKeepAliveAlert(manager: NotificationManager) {
        val now = System.currentTimeMillis()
        if (now - lastMissingKeepAliveAlertAt < MISSING_ALERT_MIN_INTERVAL_MS) return

        lastMissingKeepAliveAlertAt = now
        val contentIntent = PendingIntent.getActivity(
            this,
            1,
            Intent(this, MainActivity::class.java)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP),
            PendingIntent.FLAG_IMMUTABLE,
        )
        val notification = NotificationCompat.Builder(this, ALERT_CHANNEL_ID)
            .setContentTitle("VanillaPay 监听异常")
            .setContentText("常驻监听提示已消失，请检查应用后台运行和通知权限")
            .setSmallIcon(R.drawable.ic_notification_vanillapay)
            .setBadgeIconType(NotificationCompat.BADGE_ICON_NONE)
            .setColor(ContextCompat.getColor(this, R.color.warning))
            .setContentIntent(contentIntent)
            .setPriority(NotificationCompat.PRIORITY_HIGH)
            .setCategory(NotificationCompat.CATEGORY_ERROR)
            .setAutoCancel(true)
            .setOnlyAlertOnce(true)
            .build()
        manager.notify(ALERT_NOTIFICATION_ID, notification)
    }

    override fun onDestroy() {
        heartbeatJob?.cancel()
        super.onDestroy()
    }

    companion object {
        private const val KEEPALIVE_CHANNEL_ID = "vanillapay_keepalive"
        private const val ALERT_CHANNEL_ID = "vanillapay_keepalive_alert"
        private const val KEEPALIVE_NOTIFICATION_ID = 1
        private const val ALERT_NOTIFICATION_ID = 2
        private const val HEARTBEAT_DELAY_MS = 30_000L
        private const val MISSING_ALERT_MIN_INTERVAL_MS = 5L * 60 * 1000
    }
}
