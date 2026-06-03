package com.vanillapay.monitor.config

import android.content.Context
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
            preferences.edit().putString("server_url", value.trimEnd('/')).apply()
        }

    var deviceId: Long
        get() = preferences.getLong("device_id", 0)
        set(value) {
            preferences.edit().putLong("device_id", value).apply()
        }

    var deviceKey: String
        get() = preferences.getString("device_key", "") ?: ""
        set(value) {
            preferences.edit().putString("device_key", value).apply()
        }

    val isBound: Boolean
        get() = serverUrl.isNotEmpty() && deviceId > 0 && deviceKey.isNotEmpty()
}
