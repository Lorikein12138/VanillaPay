package com.vanillapay.monitor.work

import android.content.Context
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import com.vanillapay.monitor.BuildConfig
import com.vanillapay.monitor.config.AppConfig
import com.vanillapay.monitor.config.RuleStore
import com.vanillapay.monitor.net.ApiClient
import com.vanillapay.monitor.net.ConfigClient
import com.vanillapay.monitor.net.DeviceSigner
import com.vanillapay.monitor.net.HeartbeatPayloadBuilder
import com.vanillapay.monitor.util.ClockSync

class HeartbeatWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        val config = AppConfig(applicationContext)
        if (!config.isBound) return Result.success()
        val clock = ClockSync()
        val params = HeartbeatPayloadBuilder(DeviceSigner()).build(
            deviceId = config.deviceId,
            key = config.deviceKey,
            t = System.currentTimeMillis() / 1000,
            appVersion = BuildConfig.VERSION_NAME,
        )
        val result = runCatching { ApiClient(config.serverUrl).post("/app/heart", params) }.getOrNull()
        if (result != null && result.serverTime > 0) {
            clock.sync(result.serverTime, System.currentTimeMillis() / 1000)
        }
        if (result != null && result.parseRulesVersion > 0 && result.parseRulesVersion != RuleStore(applicationContext).version()) {
            ConfigClient(applicationContext).refresh()
        }
        return Result.success()
    }
}
