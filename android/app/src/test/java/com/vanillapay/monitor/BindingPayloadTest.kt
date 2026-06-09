package com.vanillapay.monitor

import com.vanillapay.monitor.bind.BindingPayload
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class BindingPayloadTest {
    @Test
    fun parsesValidPayload() {
        val binding = BindingPayload.parse("https://pay.example.com|5|abcdef0123456789abcdef0123456789")
        assertNotNull(binding)
        assertEquals("https://pay.example.com", binding!!.serverUrl)
        assertEquals(5L, binding.deviceId)
        assertEquals("abcdef0123456789abcdef0123456789", binding.deviceKey)
    }

    @Test
    fun rejectsMalformed() {
        assertNull(BindingPayload.parse("garbage"))
        assertNull(BindingPayload.parse("https://x|notanumber|key"))
        assertNull(BindingPayload.parse("https://x|5"))
    }

    @Test
    fun rejectsUnsafeServerUrls() {
        assertNull(BindingPayload.parse("httpx://pay.example.com|5|key"))
        assertNull(BindingPayload.parse("http://pay.example.com|5|key"))
        assertNull(BindingPayload.parse("https:///missing-host|5|key"))
    }
}
