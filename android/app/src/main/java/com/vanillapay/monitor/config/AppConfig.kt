package com.vanillapay.monitor.config

import android.content.Context
import androidx.core.content.edit
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey

class AppConfig(context: Context) {
    private val masterKey = MasterKey.Builder(context)
        .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
        .build()
    private val preferences = EncryptedSharedPreferences.create(
        context,
        "vanillapay_cfg",
        masterKey,
        EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
        EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
    )

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
}
