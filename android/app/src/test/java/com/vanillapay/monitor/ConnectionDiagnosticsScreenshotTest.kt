package com.vanillapay.monitor

import android.app.Application
import com.github.takahirom.roborazzi.captureRoboImage
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import java.io.File
import java.time.Instant

@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(application = Application::class, sdk = [35])
class ConnectionDiagnosticsScreenshotTest {
    @Test @Config(qualifiers = "zh-rCN-w360dp-h900dp-notnight-mdpi")
    fun light360() = capture(dark = false)

    @Test @Config(qualifiers = "zh-rCN-w360dp-h900dp-night-mdpi")
    fun dark360() = capture(dark = true)

    private fun capture(dark: Boolean) {
        val output = System.getProperty("monitor.screenshots")
        assumeTrue("Screenshot recording is enabled with -PmonitorScreenshots", output != null)
        val directory = File(requireNotNull(output)).apply { mkdirs() }
        val theme = if (dark) "dark" else "light"
        val time = Instant.parse("2026-11-18T03:00:00Z")
        val file = File(directory, "connection-diagnostics-$theme-360.png")
        captureRoboImage(file = file) {
            MonitorTheme(dark) {
                ConnectionDiagnosticsScreen(true, true, !dark, DiagnosticSnapshot(12, time, ""),
                    loading = false, busy = false, message = null, back = {}, copy = {}, share = {}, clear = {})
            }
        }
        assertTrue(file.length() > 0)
    }
}
