package com.vanillapay.monitor.receiver

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import com.vanillapay.monitor.service.ForegroundServiceStarter

class BootReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action == Intent.ACTION_BOOT_COMPLETED) {
            ForegroundServiceStarter.startKeepAlive(context, showToast = false)
        }
    }
}
