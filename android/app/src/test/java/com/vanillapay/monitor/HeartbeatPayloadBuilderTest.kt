package com.vanillapay.monitor

import com.vanillapay.monitor.net.DeviceSigner
import com.vanillapay.monitor.net.HeartbeatPayloadBuilder
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class HeartbeatPayloadBuilderTest {
    @Test
    fun buildsSignedHeartbeat() {
        val payload = HeartbeatPayloadBuilder(DeviceSigner()).build(7, "KEY", t = 100, appVersion = "1.0.0")
        assertEquals("7", payload["device_id"])
        assertEquals("1.0.0", payload["app_version"])
        assertEquals("100", payload["t"])
        assertTrue(payload.containsKey("sign"))
    }
}
