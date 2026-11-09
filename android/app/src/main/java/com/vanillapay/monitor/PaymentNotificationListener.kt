package com.vanillapay.monitor

import android.app.Notification
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.service.notification.NotificationListenerService
import android.service.notification.StatusBarNotification
import com.vanillapay.monitor.parse.Channel
import com.vanillapay.monitor.parse.PaymentNotice
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch

class PaymentNotificationListener : NotificationListenerService() {
    override fun onListenerConnected() {
        (application as MonitorApp).listenerConnected = true
    }

    override fun onListenerDisconnected() {
        val app = application as MonitorApp
        app.listenerConnected = false
        app.scope.launch { app.revive(rebindOnly = true) }
    }

    override fun onNotificationPosted(sbn: StatusBarNotification) {
        if (Channel.entries.none { it.packageName == sbn.packageName }) return
        val notification = sbn.notification
        val summary = (notification.flags and Notification.FLAG_GROUP_SUMMARY) != 0
        val title = notification.extras.getCharSequence(Notification.EXTRA_TITLE)?.toString().orEmpty()
        val body = (notification.extras.getCharSequence(Notification.EXTRA_BIG_TEXT)
            ?: notification.extras.getCharSequence(Notification.EXTRA_TEXT))?.toString().orEmpty()
        val notice = PaymentNotice(sbn.key, sbn.packageName, sbn.postTime, notification.`when`, title, body)
        val app = application as MonitorApp
        val delivered = app.state.value
        app.scope.launch {
            val state = if (delivered.loaded) delivered else app.state.first { it.loaded }
            val scopeAtDelivery = state.session?.takeIf { it.started && !summary }?.deviceId
            app.capture(notice, scopeAtDelivery)
        }
    }
}

class ResumeMonitoring : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != Intent.ACTION_BOOT_COMPLETED && intent.action != Intent.ACTION_MY_PACKAGE_REPLACED) return
        val app = context.applicationContext as MonitorApp
        val pending = goAsync()
        app.scope.launch {
            try {
                app.revive()
            } finally {
                pending.finish()
            }
        }
    }
}
