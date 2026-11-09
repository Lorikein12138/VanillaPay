package com.vanillapay.monitor

import android.app.Application
import com.github.takahirom.roborazzi.captureRoboImage
import com.vanillapay.monitor.parse.Channel
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
class MonitorScreenshotTest {
    @Test @Config(qualifiers = "zh-rCN-w360dp-h900dp-notnight-mdpi")
    fun light360() = capture(360, dark = false)

    @Test @Config(qualifiers = "zh-rCN-w360dp-h900dp-night-mdpi")
    fun dark360() = capture(360, dark = true)

    @Test @Config(qualifiers = "zh-rCN-w411dp-h900dp-notnight-mdpi")
    fun light411() = capture(411, dark = false)

    @Test @Config(qualifiers = "zh-rCN-w411dp-h900dp-night-mdpi")
    fun dark411() = capture(411, dark = true)

    private fun capture(width: Int, dark: Boolean) {
        val output = System.getProperty("monitor.screenshots")
        assumeTrue("Screenshot recording is enabled with -PmonitorScreenshots", output != null)
        val directory = File(requireNotNull(output)).apply { mkdirs() }
        val time = Instant.parse("2026-11-09T14:32:05Z").toEpochMilli()
        val session = Credential("https://pay.example.test/", "dev_8f3K2mQx9Lw", "", "Redmi Note 12", time - 1_800_000, 30, 0, true)
        val receipts = listOf(
            Receipt("1", session.deviceId, "alipay", 1201, time, "", status = MATCHED, detail = "VP2026110914320011"),
            Receipt("2", session.deviceId, "wxpay", 3800, time - 1_584_000, "", status = QUEUED),
            Receipt("3", session.deviceId, "wxpay", 500, time - 2_033_000, "", status = UNMATCHED),
            Receipt("4", session.deviceId, "qqpay", 9999, time - 2_280_000, "", status = FAILED, detail = "EVENT_CONFLICT"),
        )
        val diagnostics = listOf(
            Diagnostic("1", time, "wxpay", "微信支付\n微信支付收款0.01元(朋友到店)", 1),
            Diagnostic("2", time - 50_000, "alipay", "支付宝\n付款38.00元", null),
        )
        for (screen in listOf("bind", "setup", "main", "settings", "diagnostics")) {
            val theme = if (dark) "dark" else "light"
            val file = File(directory, "$screen-$theme-$width.png")
            captureRoboImage(file = file) {
                MonitorTheme(dark) {
                    when (screen) {
                        "bind" -> BindScreen("7KQF-MNT2-8WXJ", "Redmi Note 12", "https://pay.example.test", false,
                            null, {}, {}, {}, {}, {}, {})
                        "setup" -> SetupScreen(SetupChecks(true, false, false, false, false), {}, {}, {}, {}, {}, {})
                        "main" -> HomeScreen(MonitorState(true, session, true, true, time - 12_000), true,
                            setOf(Channel.Alipay, Channel.WeChat), receipts, 18, 1, time, {}, {}, {})
                        "settings" -> SettingsScreen(session, {}, {}, {}, {})
                        "diagnostics" -> DiagnosticsScreen(true, diagnostics, {}, {}, {})
                    }
                }
            }
            assertTrue("Expected nonempty screenshot ${file.name}", file.length() > 0)
        }
    }
}
