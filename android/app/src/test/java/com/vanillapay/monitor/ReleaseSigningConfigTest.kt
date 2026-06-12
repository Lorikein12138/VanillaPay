package com.vanillapay.monitor

import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.io.File

class ReleaseSigningConfigTest {
    @Test
    fun `release package version is bumped for audit fixes`() {
        val source = File("build.gradle.kts").readText()

        assertTrue(source.contains("versionCode = 9"))
        assertTrue(source.contains("""versionName = "1.1.7""""))
    }

    @Test
    fun `android 16 device builds target api 36`() {
        val source = File("build.gradle.kts").readText()

        assertTrue(source.contains("compileSdk = 36"))
        assertTrue(source.contains("targetSdk = 36"))
    }

    @Test
    fun `android 16 build uses api 36 supported android gradle plugin`() {
        val versions = File("../gradle/libs.versions.toml").readText()
        val wrapper = File("../gradle/wrapper/gradle-wrapper.properties").readText()

        assertTrue(versions.contains("""agp = "8.10.0""""))
        assertTrue(wrapper.contains("gradle-8.11.1-bin.zip"))
    }

    @Test
    fun `release signing can use stable local signing properties`() {
        val source = File("build.gradle.kts").readText()

        assertTrue(source.contains("signing.properties"))
        assertTrue(source.contains("VP_KEYSTORE"))
        assertTrue(source.contains("VP_STORE_PWD"))
        assertTrue(source.contains("VP_KEY_ALIAS"))
        assertTrue(source.contains("VP_KEY_PWD"))
    }

    @Test
    fun `encrypted preferences dependency uses stable security crypto release`() {
        val versions = File("../gradle/libs.versions.toml").readText()

        assertTrue(versions.contains("""securityCrypto = "1.1.0""""))
    }
}
