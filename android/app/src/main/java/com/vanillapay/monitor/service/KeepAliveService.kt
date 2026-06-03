package com.vanillapay.monitor.service

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.Service
import android.content.Intent
import android.os.Build
import android.os.IBinder
import androidx.core.app.NotificationCompat
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import com.vanillapay.monitor.work.HeartbeatWorker
import com.vanillapay.monitor.work.RetryWorker
import java.util.concurrent.TimeUnit

class KeepAliveService : Service() {
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
            .setSmallIcon(android.R.drawable.ic_menu_view)
            .setOngoing(true)
            .build()
        startForeground(1, notification)
        scheduleWorkers()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int = START_STICKY

    private fun scheduleWorkers() {
        val workManager = WorkManager.getInstance(this)
        workManager.enqueueUniquePeriodicWork(
            "heartbeat",
            ExistingPeriodicWorkPolicy.KEEP,
            PeriodicWorkRequestBuilder<HeartbeatWorker>(15, TimeUnit.MINUTES).build(),
        )
        workManager.enqueueUniquePeriodicWork(
            "retry",
            ExistingPeriodicWorkPolicy.KEEP,
            PeriodicWorkRequestBuilder<RetryWorker>(15, TimeUnit.MINUTES).build(),
        )
    }
}
