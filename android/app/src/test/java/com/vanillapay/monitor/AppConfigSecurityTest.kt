package com.vanillapay.monitor

import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.io.File

class AppConfigSecurityTest {
    @Test
    fun `encrypted preferences deprecation suppression is scoped to initialization`() {
        val source = File("src/main/java/com/vanillapay/monitor/config/AppConfig.kt").readText()

        assertTrue(source.contains("@Suppress(\"DEPRECATION\")"))
        assertTrue(source.contains("private fun encryptedPreferences(context: Context)"))
        assertTrue(source.contains("EncryptedSharedPreferences.create("))
        assertTrue(!source.contains("@Suppress(\"DEPRECATION\")\nclass AppConfig"))
    }
}
