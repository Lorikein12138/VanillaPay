package com.vanillapay.monitor

import com.vanillapay.monitor.net.DeviceSigner
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test
import java.security.MessageDigest

class DeviceSignerTest {
    private val signer = DeviceSigner()

    @Test
    fun matchesServerAlgorithm() {
        val params = mapOf("device_id" to "5", "channel" to "wxpay", "price" to "10.00", "t" to "100")
        val expected = md5("channel=wxpay&device_id=5&price=10.00&t=100" + "KEY")
        assertEquals(expected, signer.sign(params, "KEY"))
    }

    @Test
    fun orderIndependentAndDropsEmpty() {
        val a = signer.sign(mapOf("b" to "2", "a" to "1", "z" to ""), "K")
        val b = signer.sign(mapOf("a" to "1", "b" to "2"), "K")
        assertEquals(a, b)
    }

    private fun md5(value: String): String =
        MessageDigest.getInstance("MD5")
            .digest(value.toByteArray(Charsets.UTF_8))
            .joinToString("") { "%02x".format(it) }
}
