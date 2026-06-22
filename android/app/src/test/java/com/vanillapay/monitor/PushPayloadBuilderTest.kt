package com.vanillapay.monitor

import com.vanillapay.monitor.net.DeviceSigner
import com.vanillapay.monitor.net.PushPayloadBuilder
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class PushPayloadBuilderTest {
    @Test
    fun buildsSignedPushPayload() {
        val builder = PushPayloadBuilder(DeviceSigner())
        val payload = builder.build(
            deviceId = 5,
            key = "KEY",
            channel = "wxpay",
            amountCents = 1001,
            tradeNoDevice = "dev-1",
            t = 100,
            raw = null,
        )
        assertEquals("5", payload["device_id"])
        assertEquals("wxpay", payload["channel"])
        assertEquals("10.01", payload["price"])
        assertEquals("dev-1", payload["trade_no_device"])
        assertEquals("100", payload["t"])
        assertTrue(payload.containsKey("sign"))
        assertFalse(payload.containsKey("raw"))
    }

    @Test
    fun includesRawWhenProvided() {
        val builder = PushPayloadBuilder(DeviceSigner())
        val payload = builder.build(5, "KEY", "alipay", 8850, "d2", 200, raw = "成功收款88.50元")
        assertEquals("成功收款88.50元", payload["raw"])
    }
}
