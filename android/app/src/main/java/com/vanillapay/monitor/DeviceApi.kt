package com.vanillapay.monitor

import kotlinx.coroutines.suspendCancellableCoroutine
import okhttp3.Call
import okhttp3.Callback
import okhttp3.HttpUrl
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import org.json.JSONObject
import java.io.IOException
import java.time.Instant
import java.time.OffsetDateTime
import java.util.Locale
import java.util.concurrent.TimeUnit
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

data class Pairing(val base: String, val code: String)

fun cleanCode(raw: String): String? = raw.uppercase(Locale.ROOT)
    .filterNot { it.isWhitespace() || it == '-' }
    .takeIf { it.matches(Regex("[0-9ABCDEFGHJKMNPQRSTVWXYZ]{12}")) }

fun siteUrl(raw: String, debug: Boolean = BuildConfig.DEBUG): HttpUrl? {
    val text = raw.trim()
    if (!text.startsWith("https://", true) && !text.startsWith("http://", true)) return null
    if (text.any { it.isWhitespace() || it == '\\' }) return null
    val url = text.toHttpUrlOrNull() ?: return null
    if (url.username.isNotEmpty() || url.password.isNotEmpty() || url.encodedPath != "/" ||
        url.query != null || url.fragment != null || text.substringAfter("://").substringBefore('/').contains('@')) return null
    if (url.isHttps || (debug && isDebugHost(url.host))) return url
    return null
}

private fun isDebugHost(host: String): Boolean {
    if (host == "localhost") return true
    if (':' in host) return host == "::1" || host.startsWith("fc") || host.startsWith("fd")
    val octets = host.split('.').map { it.toIntOrNull()?.takeIf { number -> number in 0..255 } ?: return false }
    if (octets.size != 4) return false
    return octets[0] == 10 || octets[0] == 127 || octets.take(2) == listOf(192, 168) ||
        (octets[0] == 172 && octets[1] in 16..31)
}

fun pairingUri(raw: String, debug: Boolean = BuildConfig.DEBUG): Pairing? = runCatching {
    val uri = java.net.URI(raw)
    require(uri.scheme == "vanillapay" && uri.rawAuthority == "bind" && uri.path.isNullOrEmpty() && uri.rawFragment == null)
    val query = uri.rawQuery.split('&').map {
        val pair = it.split('=', limit = 2)
        require(pair.size == 2)
        pair[0] to java.net.URLDecoder.decode(pair[1], Charsets.UTF_8.name())
    }
    val values = query.toMap()
    require(query.size == 3 && values.keys == setOf("v", "base", "code") && values["v"] == "1")
    Pairing(siteUrl(values.getValue("base"), debug)!!.toString(), cleanCode(values.getValue("code"))!!)
}.getOrNull()

class ApiFailure(val status: Int, val code: String) : IOException("HTTP $status ($code)") {
    val unauthorized: Boolean get() = status == 401 && code == "DEVICE_UNAUTHORIZED"
    val retryable: Boolean get() = status == 429 || status in 500..599
}

data class Credential(
    val base: String, val deviceId: String, val token: String, val deviceName: String,
    val boundAt: Long, val interval: Int, val offsetMillis: Long, val started: Boolean = false,
) {
    override fun toString() = "Credential(deviceId=$deviceId)"
}

data class Heartbeat(val offsetMillis: Long, val interval: Int, val online: Boolean, val banned: Boolean)

data class ReportResult(val matched: Boolean, val tradeNo: String?, val reason: String?)

data class MonitorManifest(val version: String, val file: String, val sha256: String, val size: Long) {
    fun newerThan(installed: String): Boolean {
        val lhs = version.split('.').map { it.toBigInteger() }
        val rhs = installed.split('.').map { it.toBigInteger() }
        return lhs.zip(rhs).firstOrNull { (left, right) -> left != right }?.let { it.first > it.second } ?: false
    }
}

fun parseManifest(json: JSONObject): MonitorManifest? = runCatching {
    val version = json.get("version") as String
    val file = json.get("file") as String
    val hash = json.get("sha256") as String
    val size = json.get("size")
    require(size is Int || size is Long)
    val bytes = (size as Number).toLong()
    require(json.length() == 5 && version.matches(Regex("(0|[1-9][0-9]*)(\\.(0|[1-9][0-9]*)){2}")))
    require(file == "vanillapay-monitor-$version.apk" && hash.matches(Regex("[0-9a-fA-F]{64}")) && bytes > 0)
    OffsetDateTime.parse(json.get("published_at") as String)
    MonitorManifest(version, file, hash, bytes)
}.getOrNull()

class DeviceApi(private val client: OkHttpClient = OkHttpClient.Builder()
    .followRedirects(false)
    .followSslRedirects(false)
    .callTimeout(12, TimeUnit.SECONDS)
    .build()) {
    private val jsonType = "application/json; charset=utf-8".toMediaType()

    suspend fun bind(base: String, code: String, name: String): Credential {
        val start = System.currentTimeMillis()
        val data = send(base, "api/v1/device/bind", null, JSONObject()
            .put("code", code).put("device_name", name).put("app_version", BuildConfig.VERSION_NAME).toString()).getJSONObject("data")
        return Credential(base, data.getString("device_id"), data.getString("device_token"), name,
            Instant.parse(data.getString("server_time")).toEpochMilli(), interval(data), offset(data, start))
    }

    suspend fun heartbeat(session: Credential): Heartbeat {
        val start = System.currentTimeMillis()
        val data = send(session.base, "api/v1/device/heartbeat", session.token, JSONObject()
            .put("app_version", BuildConfig.VERSION_NAME).put("device_time", Instant.ofEpochMilli(start).toString()).toString()).getJSONObject("data")
        return Heartbeat(offset(data, start), interval(data), data.getBoolean("online"), data.getBoolean("merchant_banned"))
    }

    suspend fun report(session: Credential, payload: String): ReportResult {
        val data = send(session.base, "api/v1/device/receipts", session.token, payload).getJSONObject("data")
        return ReportResult(data.getString("match_state") == "matched", data.optionalString("trade_no"), data.optionalString("unmatched_reason"))
    }

    suspend fun unbind(session: Credential) {
        send(session.base, "api/v1/device/unbind", session.token, "")
    }

    suspend fun latest(base: String): MonitorManifest? = parseManifest(send(base, "downloads/latest.json", null, null))

    private fun interval(data: JSONObject) = data.getInt("heartbeat_interval_seconds").also { require(it > 0) }

    private fun offset(data: JSONObject, start: Long): Long {
        val end = System.currentTimeMillis()
        return Instant.parse(data.getString("server_time")).toEpochMilli() - (start + (end - start) / 2)
    }

    private suspend fun send(base: String, path: String, token: String?, payload: String?): JSONObject {
        val url = requireNotNull(siteUrl(base)).newBuilder().addPathSegments(path).build()
        val request = Request.Builder().url(url).apply {
            if (token != null) header("Authorization", "Bearer $token")
            if (payload != null) post(payload.toRequestBody(if (payload.isEmpty()) null else jsonType))
        }.build()
        return suspendCancellableCoroutine { continuation ->
            val call = client.newCall(request)
            continuation.invokeOnCancellation { call.cancel() }
            call.enqueue(object : Callback {
                override fun onFailure(call: Call, e: IOException) = continuation.resumeWithException(e)

                override fun onResponse(call: Call, response: Response) {
                    val result = runCatching {
                        response.use {
                            if (it.code == 204) return@use JSONObject()
                            val text = it.peekBody(65_537).string()
                            if (!it.isSuccessful) {
                                val code = runCatching { JSONObject(text).getJSONObject("error").getString("code") }.getOrDefault("")
                                throw ApiFailure(it.code, code)
                            }
                            if (text.length > 65_536) throw IOException("Response too large")
                            JSONObject(text)
                        }
                    }
                    continuation.resumeWith(result)
                }
            })
        }
    }
}

private fun JSONObject.optionalString(name: String) = if (isNull(name)) null else getString(name)
