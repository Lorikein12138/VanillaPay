package com.vanillapay.monitor.net

import android.content.Context
import com.vanillapay.monitor.BuildConfig
import com.vanillapay.monitor.config.AppConfig
import com.vanillapay.monitor.config.RuleStore
import okhttp3.Request
import java.net.URI

class ConfigClient(private val context: Context) {
    fun refresh() {
        val config = AppConfig(context)
        if (!config.isBound) return
        val host = runCatching { URI(config.serverUrl).host.orEmpty() }.getOrDefault("")
        val http = PinnedClientFactory.create(
            host = BuildConfig.CERT_PIN_HOST.ifBlank { host },
            pinSha256 = BuildConfig.CERT_PIN_SHA256.ifBlank { null },
        )
        val request = Request.Builder()
            .url(config.serverUrl.trimEnd('/') + "/app/config")
            .get()
            .build()
        runCatching {
            http.newCall(request).execute().use { response ->
                val body = response.body?.string().orEmpty()
                if (response.isSuccessful && body.isNotEmpty()) {
                    RuleStore(context).save(body)
                }
            }
        }
    }
}
