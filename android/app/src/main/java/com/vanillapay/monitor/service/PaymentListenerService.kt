package com.vanillapay.monitor.service

import android.app.Notification
import android.content.ComponentName
import android.os.Bundle
import android.service.notification.NotificationListenerService
import android.service.notification.StatusBarNotification
import android.util.Log
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
        ForegroundServiceStarter.startKeepAlive(this, showToast = false)
    }

    override fun onListenerConnected() {
        super.onListenerConnected()
        ForegroundServiceStarter.startKeepAlive(this, showToast = false)
    }

    override fun onNotificationPosted(sbn: StatusBarNotification) {
        val notification = sbn.notification ?: return
        val extras = notification.extras ?: return
        val title = extras.getCharSequence(Notification.EXTRA_TITLE)?.toString().orEmpty()
        val text = collectText(notification, extras)
        if (title.isBlank() && text.isBlank()) return
        val parser = NotificationParser(RuleStore(applicationContext).current().rules)
        val parsed = parser.parse(sbn.packageName, title, text)
        if (parsed == null) {
            // Surface payment-app notifications we failed to recognize so the parse
            // rules can be tuned later (visible via `adb logcat -s $TAG`).
            if (isPaymentPackage(sbn.packageName)) {
                Log.i(TAG, "unrecognized ${sbn.packageName} | title=[$title] text=[$text]")
            }
            return
        }
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
        ForegroundServiceStarter.startKeepAlive(this, showToast = false)
    }

    override fun onDestroy() {
        scope.cancel()
        super.onDestroy()
    }

    /**
     * Gather every text-bearing field of a notification, not just title/text. Some phone
     * vendors (and WeChat's grouped notifications) put the amount in big-text, sub/info/summary
     * text, the inbox-style text lines, or the legacy ticker — reading only EXTRA_TEXT misses them.
     */
    private fun collectText(notification: Notification, extras: Bundle): String {
        val parts = ArrayList<CharSequence?>(8)
        parts.add(extras.getCharSequence(Notification.EXTRA_TEXT))
        parts.add(extras.getCharSequence(Notification.EXTRA_BIG_TEXT))
        parts.add(extras.getCharSequence(Notification.EXTRA_SUB_TEXT))
        parts.add(extras.getCharSequence(Notification.EXTRA_INFO_TEXT))
        parts.add(extras.getCharSequence(Notification.EXTRA_SUMMARY_TEXT))
        extras.getCharSequenceArray(Notification.EXTRA_TEXT_LINES)?.forEach { parts.add(it) }
        parts.add(notification.tickerText)
        return parts.asSequence()
            .filterNotNull()
            .map { it.toString().trim() }
            .filter { it.isNotEmpty() }
            .distinct()
            .joinToString(" ")
    }

    private fun isPaymentPackage(packageName: String): Boolean =
        packageName == "com.tencent.mm" || packageName == "com.eg.android.AlipayGphone"

    private companion object {
        const val TAG = "VanillaPayListener"
    }
}
