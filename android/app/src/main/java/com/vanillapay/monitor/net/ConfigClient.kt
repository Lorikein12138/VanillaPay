package com.vanillapay.monitor.net

import android.content.Context
import com.vanillapay.monitor.BuildConfig
import com.vanillapay.monitor.config.AppConfig
import com.vanillapay.monitor.config.RuleStore
import com.vanillapay.monitor.parse.RuleParser
import com.vanillapay.monitor.util.ClockSync
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.Request
import java.net.URI

class ConfigClient(private val context: Context, private val clock: ClockSync = ClockSync()) {
    fun refresh() {
        val config = AppConfig(context)
        if (!config.isBound) return
        val host = runCatching { URI(config.serverUrl).host.orEmpty() }.getOrDefault("")
        val http = PinnedClientFactory.create(
            host = BuildConfig.CERT_PIN_HOST.ifBlank { host },
            pinSha256 = BuildConfig.CERT_PIN_SHA256.ifBlank { null },
        )
        val params = mutableMapOf(
            "device_id" to config.deviceId.toString(),
            "t" to clock.now().toString(),
        )
        params["sign"] = DeviceSigner().sign(params, config.deviceKey)
        val urlBuilder = (config.serverUrl.trimEnd('/') + "/app/config").toHttpUrl().newBuilder()
        params.forEach { (key, value) -> urlBuilder.addQueryParameter(key, value) }
        val request = Request.Builder()
            .url(urlBuilder.build())
            .get()
            .build()
        runCatching {
            http.newCall(request).execute().use { response ->
                val body = response.body?.string().orEmpty()
                if (response.isSuccessful && RuleParser.parse(body).rules.isNotEmpty()) {
                    RuleStore(context).save(body)
                }
            }
        }
    }
}
