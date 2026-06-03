package com.vanillapay.monitor.service

import android.content.Context
import com.vanillapay.monitor.config.AppConfig
import com.vanillapay.monitor.data.AppDatabase
import com.vanillapay.monitor.data.PushRecord
import com.vanillapay.monitor.net.ApiClient
import com.vanillapay.monitor.net.Backoff
import com.vanillapay.monitor.net.DeviceSigner
import com.vanillapay.monitor.net.PushPayloadBuilder
import com.vanillapay.monitor.util.ClockSync
import com.vanillapay.monitor.util.RawHash

class Reporter(context: Context, private val clock: ClockSync) {
    private val appContext = context.applicationContext
    private val config = AppConfig(appContext)
    private val dao = AppDatabase.get(appContext).pushDao()
    private val payloadBuilder = PushPayloadBuilder(DeviceSigner())

    suspend fun enqueue(
        packageName: String,
        channel: String,
        amountCents: Long,
        rawText: String,
        postTime: Long,
    ) {
        val hash = RawHash.of(packageName, rawText, postTime)
        if (dao.countByHash(hash) > 0) return
        dao.insert(
            PushRecord(
                rawHash = hash,
                channel = channel,
                amountCents = amountCents,
                t = clock.now(),
                raw = rawText,
            ),
        )
        drain()
    }

    suspend fun drain() {
        if (!config.isBound) return
        val api = ApiClient(config.serverUrl)
        val nowMillis = System.currentTimeMillis()
        for (record in dao.due(nowMillis)) {
            val params = payloadBuilder.build(
                deviceId = config.deviceId,
                key = config.deviceKey,
                channel = record.channel,
                amountCents = record.amountCents,
                tradeNoDevice = record.rawHash,
                t = clock.now(),
                raw = record.raw,
            )
            val result = runCatching { api.post("/app/push", params) }.getOrNull()
            if (result?.ok == true) {
                if (result.serverTime > 0) {
                    clock.sync(result.serverTime, System.currentTimeMillis() / 1000)
                }
                dao.update(record.copy(status = "sent"))
            } else {
                val attempts = record.attempts + 1
                dao.update(
                    record.copy(
                        status = "failed",
                        attempts = attempts,
                        nextRetryAt = nowMillis + Backoff.delaySeconds(attempts) * 1000,
                    ),
                )
            }
        }
    }
}
