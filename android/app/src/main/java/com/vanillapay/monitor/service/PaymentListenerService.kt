package com.vanillapay.monitor.service

import android.app.Notification
import android.content.ComponentName
import android.service.notification.NotificationListenerService
import android.service.notification.StatusBarNotification
import com.vanillapay.monitor.config.RuleStore
import com.vanillapay.monitor.parse.NotificationParser
import com.vanillapay.monitor.util.ClockSync
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch

class PaymentListenerService : NotificationListenerService() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val clock = ClockSync()
    private lateinit var reporter: Reporter

    override fun onCreate() {
        super.onCreate()
        reporter = Reporter(applicationContext, clock)
    }

    override fun onNotificationPosted(sbn: StatusBarNotification) {
        val extras = sbn.notification?.extras ?: return
        val title = extras.getCharSequence(Notification.EXTRA_TITLE)?.toString().orEmpty()
        val text = (
            extras.getCharSequence(Notification.EXTRA_BIG_TEXT)
                ?: extras.getCharSequence(Notification.EXTRA_TEXT)
            )?.toString().orEmpty()
        val parser = NotificationParser(RuleStore(applicationContext).current().rules)
        val parsed = parser.parse(sbn.packageName, title, text) ?: return
        scope.launch {
            reporter.enqueue(
                packageName = sbn.packageName,
                channel = parsed.channel,
                amountCents = parsed.amountCents,
                rawText = "$title $text".trim(),
                postTime = sbn.postTime,
            )
        }
    }

    override fun onListenerDisconnected() {
        requestRebind(ComponentName(this, javaClass))
    }

    override fun onDestroy() {
        scope.cancel()
        super.onDestroy()
    }
}
