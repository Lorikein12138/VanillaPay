package com.vanillapay.monitor

import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

@RunWith(RobolectricTestRunner::class)
@Config(application = android.app.Application::class, sdk = [35])
class DeviceApiTest {
    @Test fun onlyDebugPrivateAddressesAllowCleartextAndQrCannotOverrideUrlRules() {
        assertNull(siteUrl("http://localhost", debug = false))
        assertNull(siteUrl("http://example.test", debug = true))
        assertNotNull(siteUrl("http://10.0.2.2:8080", debug = true))
        assertNotNull(siteUrl("http://192.168.1.4", debug = true))
        assertNull(siteUrl("https://example.test@elsewhere.test", debug = true))
        assertNull(siteUrl("https://example.test/path", debug = true))
        val qr = "vanillapay://bind?v=1&base=https%3A%2F%2Fexample.test&code=ABCD+EFGH-JKMN"
        assertEquals(Pairing("https://example.test/", "ABCDEFGHJKMN"), pairingUri(qr, false))
        assertNull(pairingUri("$qr&code=ZZZZZZZZZZZZ", false))
        assertNull(pairingUri(qr.replace("https", "http"), false))
    }

    @Test fun checksManifestShapeAndNumericVersion() {
        val json = JSONObject().put("version", "1.10.0").put("file", "vanillapay-monitor-1.10.0.apk")
            .put("sha256", "a".repeat(64)).put("size", 123).put("published_at", "2026-11-09T12:00:00Z")
        assertTrue(parseManifest(json)!!.newerThan("1.9.9"))
        assertFalse(parseManifest(json)!!.newerThan("1.10.0"))
        assertNull(parseManifest(json.put("file", "//other.test/monitor.apk")))
    }

    @Test fun unbindSendsNoBodyAndDoesNotFollowRedirectsWithTheBearerToken() = runBlocking {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setResponseCode(302).addHeader("Location", server.url("/elsewhere")))
            val session = Credential(server.url("/").toString(), "device-a", "fixture", "Phone", 0, 30, 0)
            val api = DeviceApi()
            assertEquals(302, (runCatching { api.unbind(session) }.exceptionOrNull() as ApiFailure).status)
            val request = server.takeRequest()
            assertEquals("/api/v1/device/unbind", request.path)
            assertEquals("POST", request.method)
            assertEquals("Bearer fixture", request.getHeader("Authorization"))
            assertEquals(0L, request.bodySize)
            assertEquals(1, server.requestCount)
            server.enqueue(MockResponse().setResponseCode(204))
            api.unbind(session)
        }
    }
}
