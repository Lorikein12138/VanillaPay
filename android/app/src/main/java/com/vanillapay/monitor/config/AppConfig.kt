package com.vanillapay.monitor.config

import android.content.Context
import androidx.core.content.edit

class AppConfig(context: Context) {
    private val preferences = encryptedPreferences(context)

    var serverUrl: String
        get() = preferences.getString("server_url", "") ?: ""
        set(value) {
            preferences.edit { putString("server_url", value.trimEnd('/')) }
        }

    var deviceId: Long
        get() = preferences.getLong("device_id", 0)
        set(value) {
            preferences.edit { putLong("device_id", value) }
        }

    var deviceKey: String
        get() = preferences.getString("device_key", "") ?: ""
        set(value) {
            preferences.edit { putString("device_key", value) }
        }

    var autostartConfirmed: Boolean
        get() = preferences.getBoolean("autostart_confirmed", false)
        set(value) {
            preferences.edit { putBoolean("autostart_confirmed", value) }
        }

    var lastHeartbeatAt: Long
        get() = preferences.getLong("last_heartbeat_at", 0L)
        set(value) {
            preferences.edit { putLong("last_heartbeat_at", value) }
        }

    var merchantPid: String
        get() = preferences.getString("merchant_pid", "") ?: ""
        set(value) {
            preferences.edit { putString("merchant_pid", value) }
        }

    val isBound: Boolean
        get() = serverUrl.isNotEmpty() && deviceId > 0 && deviceKey.isNotEmpty()

    @Suppress("DEPRECATION")
    private fun encryptedPreferences(context: Context) =
        androidx.security.crypto.EncryptedSharedPreferences.create(
            context,
            "vanillapay_cfg",
            androidx.security.crypto.MasterKey.Builder(context)
                .setKeyScheme(androidx.security.crypto.MasterKey.KeyScheme.AES256_GCM)
                .build(),
            androidx.security.crypto.EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
            androidx.security.crypto.EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
        )
}
