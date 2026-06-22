package com.vanillapay.monitor.bind

import java.net.URI

data class BindingPayload(
    val serverUrl: String,
    val deviceId: Long,
    val deviceKey: String,
) {
    companion object {
        fun parse(text: String): BindingPayload? {
            val parts = text.trim().split("|")
            if (parts.size != 3) return null
            val serverUrl = parts[0].trim().trimEnd('/')
            val deviceId = parts[1].toLongOrNull() ?: return null
            val deviceKey = parts[2].trim()
            val uri = runCatching { URI(serverUrl) }.getOrNull() ?: return null
            if (uri.scheme != "https" || uri.host.isNullOrBlank() || deviceId <= 0 || deviceKey.isEmpty()) return null
            return BindingPayload(serverUrl, deviceId, deviceKey)
        }
    }
}
