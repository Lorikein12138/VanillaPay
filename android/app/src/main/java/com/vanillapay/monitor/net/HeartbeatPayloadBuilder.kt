package com.vanillapay.monitor.net

class HeartbeatPayloadBuilder(private val signer: DeviceSigner) {
    fun build(deviceId: Long, key: String, t: Long, appVersion: String): Map<String, String> {
        val params = linkedMapOf(
            "device_id" to deviceId.toString(),
            "app_version" to appVersion,
            "t" to t.toString(),
        )
        params["sign"] = signer.sign(params, key)
        return params
    }
}
