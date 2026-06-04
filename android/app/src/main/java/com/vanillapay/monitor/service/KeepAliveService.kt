package com.vanillapay.monitor.service

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.Service
import android.content.Intent
import android.graphics.BitmapFactory
import android.os.Build
import android.os.IBinder
import androidx.core.app.NotificationCompat
import androidx.core.content.ContextCompat
import com.vanillapay.monitor.R
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

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onCreate() {
        super.onCreate()
        val channelId = "vanillapay_keepalive"
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val manager = getSystemService(NotificationManager::class.java)
            manager.createNotificationChannel(
                NotificationChannel(channelId, "监听服务", NotificationManager.IMPORTANCE_LOW),
            )
        }
        val notification: Notification = NotificationCompat.Builder(this, channelId)
            .setContentTitle("VanillaPay 监听运行中")
            .setContentText("正在监听微信 / 支付宝到账通知")
            .setSmallIcon(R.drawable.ic_stat_monitor)
            .setLargeIcon(BitmapFactory.decodeResource(resources, R.drawable.ic_notification_large))
            .setColor(ContextCompat.getColor(this, R.color.brand_primary))
            .setOngoing(true)
            .build()
        startForeground(1, notification)
        scheduleWorkers()
        startHeartbeatLoop()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int = START_STICKY

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
                HeartbeatReporter(applicationContext).send()
                delay(30_000L)
            }
        }
    }

    override fun onDestroy() {
        heartbeatJob?.cancel()
        super.onDestroy()
    }
}
