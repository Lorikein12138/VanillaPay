package com.vanillapay.monitor

import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.io.File

class ReleaseSigningConfigTest {
    @Test
    fun `release signing can use stable local signing properties`() {
        val source = File("build.gradle.kts").readText()

        assertTrue(source.contains("signing.properties"))
        assertTrue(source.contains("VP_KEYSTORE"))
        assertTrue(source.contains("VP_STORE_PWD"))
        assertTrue(source.contains("VP_KEY_ALIAS"))
        assertTrue(source.contains("VP_KEY_PWD"))
    }
}
