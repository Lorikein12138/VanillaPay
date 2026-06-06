package com.vanillapay.monitor.net

import com.vanillapay.monitor.BuildConfig
import okhttp3.FormBody
import okhttp3.Request
import org.json.JSONObject
import java.net.URI

data class ApiResult(
    val ok: Boolean,
    val code: Int,
    val matched: Boolean,
    val serverTime: Long,
    val parseRulesVersion: Int,
    val pid: String = "",
)

class ApiClient(private val baseUrl: String) {
    private val host = runCatching { URI(baseUrl).host.orEmpty() }.getOrDefault("")
    private val http = PinnedClientFactory.create(
        host = BuildConfig.CERT_PIN_HOST.ifBlank { host },
        pinSha256 = BuildConfig.CERT_PIN_SHA256.ifBlank { null },
    )

    fun post(path: String, params: Map<String, String>): ApiResult {
        val form = FormBody.Builder().apply {
            params.forEach { (key, value) -> add(key, value) }
        }.build()
        val request = Request.Builder()
            .url(baseUrl.trimEnd('/') + path)
            .post(form)
            .build()
        http.newCall(request).execute().use { response ->
            val body = response.body?.string().orEmpty()
            val json = runCatching { JSONObject(body) }.getOrNull()
            val code = json?.optInt("code", -1) ?: -1
            val config = json?.optJSONObject("config")
            val version = config?.optInt("parse_rules_version", 0)
                ?: json?.optInt("parse_rules_version", 0)
                ?: 0
            return ApiResult(
                ok = response.isSuccessful && code == 1,
                code = code,
                matched = json?.optBoolean("matched", false) ?: false,
                serverTime = json?.optLong("server_time", 0L) ?: 0L,
                parseRulesVersion = version,
                pid = json?.optString("pid", "") ?: "",
            )
        }
    }
}
