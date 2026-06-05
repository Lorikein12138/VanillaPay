package com.vanillapay.monitor.net

import android.content.Context
import com.vanillapay.monitor.BuildConfig
import com.vanillapay.monitor.config.AppConfig
import com.vanillapay.monitor.config.RuleStore
import com.vanillapay.monitor.util.ClockSync

class HeartbeatReporter(private val context: Context) {
    /** @return true when the heartbeat round-trip reached the server. */
    fun send(): Boolean {
        val config = AppConfig(context)
        if (!config.isBound) return false

        val clock = ClockSync()
        val params = HeartbeatPayloadBuilder(DeviceSigner()).build(
            deviceId = config.deviceId,
            key = config.deviceKey,
            t = System.currentTimeMillis() / 1000,
            appVersion = BuildConfig.VERSION_NAME,
        )
        val result = runCatching { ApiClient(config.serverUrl).post("/app/heart", params) }.getOrNull()
            ?: return false

        if (result.serverTime > 0) {
            clock.sync(result.serverTime, System.currentTimeMillis() / 1000)
        }
        if (result.parseRulesVersion > 0 && result.parseRulesVersion != RuleStore(context).version()) {
            ConfigClient(context).refresh()
        }
        config.lastHeartbeatAt = System.currentTimeMillis()
        return true
    }
}
