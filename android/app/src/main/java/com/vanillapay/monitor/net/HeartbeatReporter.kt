package com.vanillapay.monitor.net

import android.content.Context
import android.content.Intent
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
        if (result.pid.isNotBlank()) {
            config.merchantPid = result.pid
        }
        config.lastHeartbeatAt = System.currentTimeMillis()
        context.sendBroadcast(Intent(ACTION_HEARTBEAT).setPackage(context.packageName))
        return true
    }

    companion object {
        const val ACTION_HEARTBEAT = "com.vanillapay.monitor.action.HEARTBEAT"
    }
}
