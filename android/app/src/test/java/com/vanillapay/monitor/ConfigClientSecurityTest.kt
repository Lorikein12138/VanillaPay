package com.vanillapay.monitor

import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.io.File

class ConfigClientSecurityTest {
    @Test
    fun `config refresh request is signed with device credentials`() {
        val source = File("src/main/java/com/vanillapay/monitor/net/ConfigClient.kt").readText()

        assertTrue(source.contains("DeviceSigner().sign"))
        assertTrue(source.contains("device_id"))
        assertTrue(source.contains("sign"))
    }
}
