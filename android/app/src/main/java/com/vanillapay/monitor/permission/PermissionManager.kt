package com.vanillapay.monitor.permission

import android.content.Context
import android.os.PowerManager
import android.provider.Settings
import com.vanillapay.monitor.config.AppConfig

/** Central, UI-free judgment of the three permissions the app gates on. */
object PermissionManager {
    fun isNotificationListenerEnabled(context: Context): Boolean {
        val flat = Settings.Secure.getString(context.contentResolver, "enabled_notification_listeners")
        return !flat.isNullOrEmpty() && flat.contains(context.packageName)
    }

    fun isIgnoringBatteryOptimizations(context: Context): Boolean {
        val pm = context.getSystemService(PowerManager::class.java) ?: return false
        return pm.isIgnoringBatteryOptimizations(context.packageName)
    }

    fun isAutostartConfirmed(context: Context): Boolean = AppConfig(context).autostartConfirmed

    fun allGranted(context: Context): Boolean =
        isNotificationListenerEnabled(context) &&
            isIgnoringBatteryOptimizations(context) &&
            isAutostartConfirmed(context)
}
