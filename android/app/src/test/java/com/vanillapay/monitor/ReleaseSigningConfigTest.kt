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
    fun `android build compiles and targets latest installed api`() {
        val source = File("build.gradle.kts").readText()

        assertTrue(source.contains("compileSdk = 37"))
        assertTrue(source.contains("targetSdk = 37"))
    }

    @Test
    fun `android build uses current stable android gradle plugin toolchain`() {
        val versions = File("../gradle/libs.versions.toml").readText()
        val wrapper = File("../gradle/wrapper/gradle-wrapper.properties").readText()

        assertTrue(versions.contains("""agp = "9.2.1""""))
        assertTrue(versions.contains("""ksp = "2.3.9""""))
        assertTrue(!versions.contains("kotlin-android"))
        assertTrue(wrapper.contains("gradle-9.5.1-bin.zip"))
    }

    @Test
    fun `androidx dependencies use lint recommended stable versions`() {
        val versions = File("../gradle/libs.versions.toml").readText()

        assertTrue(versions.contains("""coreKtx = "1.19.0""""))
        assertTrue(versions.contains("""appcompat = "1.7.1""""))
        assertTrue(versions.contains("""material = "1.14.0""""))
        assertTrue(versions.contains("""lifecycle = "2.10.0""""))
        assertTrue(versions.contains("""room = "2.8.4""""))
        assertTrue(versions.contains("""workManager = "2.11.2""""))
        assertTrue(versions.contains("""camera = "1.6.1""""))
        assertTrue(versions.contains("""okhttp = "5.4.0""""))
        assertTrue(versions.contains("""coroutines = "1.11.0""""))
        assertTrue(versions.contains("""junit = "6.1.0""""))
        assertTrue(versions.contains("""junitPlatform = "6.1.0""""))
        assertTrue(versions.contains("""json = "20260522""""))
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
