package com.vanillapay.monitor.net

import com.vanillapay.monitor.Money

class PushPayloadBuilder(private val signer: DeviceSigner) {
    fun build(
        deviceId: Long,
        key: String,
        channel: String,
        amountCents: Long,
        tradeNoDevice: String,
        t: Long,
        raw: String?,
    ): Map<String, String> {
        val params = linkedMapOf(
            "device_id" to deviceId.toString(),
            "channel" to channel,
            "price" to Money.format(amountCents),
            "trade_no_device" to tradeNoDevice,
            "t" to t.toString(),
        )
        if (!raw.isNullOrEmpty()) {
            params["raw"] = raw
        }
        params["sign"] = signer.sign(params, key)
        return params
    }
}
